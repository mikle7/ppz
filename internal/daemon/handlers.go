package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nkeys"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/clock"
	"github.com/pipescloud/ppz/internal/envelope"
	"github.com/pipescloud/ppz/internal/natsubj"
)

// natsObserveOptions returns the connection-state observation handlers
// (Phase 0 of agent hardening). Both connect helpers below splat these
// onto every nats.Connect so disconnect / reconnect / closed transitions
// land in the daemon's NATSEventRing — surfaced by `ppz status` and
// `ppz diag`.
//
// Phase 0 is observe-only — no behaviour change. We do NOT pass
// nats.MaxReconnects(-1) or jitter options here; those are Phase 1
// fixes scoped against the data this instrumentation produces.
//
// Caller passes the ring rather than the daemon to keep the helpers
// importable from anywhere they're needed without a circular reference
// back to the daemon struct. ring may be nil (tests / paths without
// observability), in which case the handlers no-op.
func natsObserveOptions(ring *NATSEventRing) []nats.Option {
	record := func(typ, reason string) {
		if ring == nil {
			return
		}
		ring.Append(typ, reason, time.Now())
	}
	return []nats.Option{
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			reason := ""
			if err != nil {
				reason = err.Error()
			}
			record("disconnect", reason)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			reason := ""
			if nc != nil {
				if u := nc.ConnectedUrl(); u != "" {
					reason = u
				}
			}
			record("reconnect", reason)
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			reason := ""
			if nc != nil {
				if e := nc.LastError(); e != nil {
					reason = e.Error()
				}
			}
			record("closed", reason)
		}),
	}
}

// connectNATSWithJWT connects to NATS at url, authenticating with the
// supplied User JWT + seed (Phase 3). Static — kept for legacy code
// paths that don't yet flow through the refresh loop.
func connectNATSWithJWT(url, userJWT, userSeed string, ring *NATSEventRing) (*nats.Conn, error) {
	if userJWT == "" || userSeed == "" {
		return nil, errors.New("connectNATSWithJWT: missing nats user jwt/seed in credentials")
	}
	kp, err := nkeys.FromSeed([]byte(userSeed))
	if err != nil {
		return nil, fmt.Errorf("parse user seed: %w", err)
	}
	opts := append([]nats.Option{
		nats.UserJWT(
			func() (string, error) { return userJWT, nil },
			func(nonce []byte) ([]byte, error) { return kp.Sign(nonce) },
		),
	}, natsObserveOptions(ring)...)
	return nats.Connect(url, opts...)
}

// connectNATSWithRefresh connects to NATS at url, reading the live
// User JWT + seed from the supplied RefreshLoop on every (re)connect.
// nats.go calls the callbacks once per connection establishment; if
// the refresh loop has rotated credentials in the meantime, the next
// reconnect picks up the fresh values.
func connectNATSWithRefresh(url string, r *RefreshLoop, ring *NATSEventRing) (*nats.Conn, error) {
	jwt, seed := r.Current()
	if jwt == "" || seed == "" {
		return nil, errors.New("connectNATSWithRefresh: refresh loop has no credentials yet")
	}
	opts := append([]nats.Option{
		nats.UserJWT(
			func() (string, error) {
				j, _ := r.Current()
				if j == "" {
					return "", errors.New("no jwt")
				}
				return j, nil
			},
			func(nonce []byte) ([]byte, error) {
				_, s := r.Current()
				if s == "" {
					return nil, errors.New("no seed")
				}
				kp, err := nkeys.FromSeed([]byte(s))
				if err != nil {
					return nil, err
				}
				return kp.Sign(nonce)
			},
		),
	}, natsObserveOptions(ring)...)
	return nats.Connect(url, opts...)
}

// authExchangeRefresh is the RefreshFn we register with RefreshLoop.
// On every fire it re-runs POST /api/v1/auth/exchange with the
// daemon's bearer for the supplied accountID, and returns the new
// (jwt, seed, exp). 401 → ErrUnauthorized so the loop stops + the
// daemon's loginCheck flips to invalid.
func (d *Daemon) authExchangeRefresh(ctx context.Context, accountID string) (string, string, int64, error) {
	creds, ok := d.State.Credentials()
	if !ok {
		return "", "", 0, ErrUnauthorized
	}
	// Phase 4: thread the persisted AccountID so the refresh stays bound to
	// the org the user is currently switched to. Empty AccountID falls back
	// to the server's "first owned org" default — preserves behaviour
	// for daemons that haven't yet switched.
	body, _ := json.Marshal(cliproto.AuthExchangeRequest{APIKey: creds.APIKey, AccountID: d.State.AccountID()})
	req, err := http.NewRequestWithContext(ctx, "POST", creds.URL+"/api/v1/auth/exchange", bytes.NewReader(body))
	if err != nil {
		return "", "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.HTTP.Do(req)
	if err != nil {
		return "", "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return "", "", 0, ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", 0, fmt.Errorf("auth/exchange: HTTP %d", resp.StatusCode)
	}
	var ex cliproto.AuthExchangeReply
	if err := json.NewDecoder(resp.Body).Decode(&ex); err != nil {
		return "", "", 0, err
	}
	// Persist the refreshed creds so a daemon restart picks them up
	// without an immediate /auth/exchange round-trip.
	creds.NATSUserJWT = ex.NATSUserJWT
	creds.NATSUserSeed = ex.NATSUserSeed
	_ = d.State.SetLogin(*creds, ex.AccountID, ex.AccountName, keyPrefix(creds.APIKey))
	return ex.NATSUserJWT, ex.NATSUserSeed, ex.ExpiresAt.Unix(), nil
}

// startRefreshLoop swaps d.Refresh for a fresh RefreshLoop tied to
// the supplied initial credentials. Idempotent — stops any existing
// loop first.
func (d *Daemon) startRefreshLoop(accountID, jwt, seed string, expUnix int64) {
	if d.Refresh != nil {
		d.Refresh.Stop()
	}
	d.Refresh = &RefreshLoop{
		AccountID:   accountID,
		Refresh: d.authExchangeRefresh,
		OnUnauthorized: func(string) {
			d.State.SetLoginCheck(cliproto.LoginCheckInvalid)
		},
	}
	// context.Background — refresh loop outlives any single IPC
	// request. Stop() (called on Login replacement / Logout) is the
	// only way it ends.
	_ = d.Refresh.Start(context.Background(), jwt, seed, expUnix)
}

// handleLogin: POSTs the API key to /api/v1/auth/exchange, stores credentials
// + org ID, and (best-effort) opens a NATS connection.
func (d *Daemon) handleLogin(ctx context.Context, conn net.Conn, params json.RawMessage) {
	var req cliproto.LoginRequest
	if err := json.Unmarshal(params, &req); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()})
		return
	}
	body, _ := json.Marshal(cliproto.AuthExchangeRequest{APIKey: req.APIKey})
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", req.URL+"/api/v1/auth/exchange", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := d.HTTP.Do(httpReq)
	if err != nil {
		writeIPCErr(conn, cliproto.New(cliproto.EServerUnreachable))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		writeIPCErr(conn, cliproto.New(cliproto.EInvalidAPIKey))
		return
	}
	if resp.StatusCode != http.StatusOK {
		writeIPCErr(conn, &cliproto.Error{Code: "E_HTTP", Message: resp.Status})
		return
	}
	var ex cliproto.AuthExchangeReply
	if err := json.NewDecoder(resp.Body).Decode(&ex); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()})
		return
	}

	creds := Credentials{
		URL:          req.URL,
		APIKey:       req.APIKey,
		NATSUserJWT:  ex.NATSUserJWT,
		NATSUserSeed: ex.NATSUserSeed,
	}
	prefix := keyPrefix(req.APIKey)
	if err := d.State.SetLogin(creds, ex.AccountID, ex.AccountName, prefix); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}

	// PPZ_NATS_URL on the daemon overrides what the server told us. Used
	// when running the daemon outside compose: the server hands out
	// "nats://ppz-server:4222" (correct for in-compose clients) but a host
	// client needs "nats://localhost:4222" instead.
	natsURL := ex.NATSURL
	if v := os.Getenv("PPZ_NATS_URL"); v != "" {
		natsURL = v
	}
	d.NATSURL = natsURL

	// Connect (or reconnect) to NATS so broadcast/list paths can use
	// it. Phase 3.5: connection reads its JWT/seed from the refresh
	// loop on every (re)connect, so when the loop rotates creds the
	// next NATS reconnect picks them up automatically.
	if d.NC != nil {
		d.NC.Close()
		d.NC = nil
	}
	d.startRefreshLoop(ex.AccountID, ex.NATSUserJWT, ex.NATSUserSeed, ex.ExpiresAt.Unix())
	if nc, err := connectNATSWithRefresh(natsURL, d.Refresh, d.NATSEvents); err == nil {
		d.NC = nc
	}

	writeIPC(conn, cliproto.LoginReply{URL: req.URL, KeyPrefix: prefix, AccountID: ex.AccountID})
}

// ensureNATS establishes the daemon's NATS connection from stored
// credentials. Login does this proactively, but a daemon restarted via
// `ppz kill` + `ppz daemon` only reloads creds from disk — d.NATSURL is
// in-memory state that doesn't survive. We rebuild it here on demand by
// re-calling /auth/exchange.
func (d *Daemon) ensureNATS(ctx context.Context) error {
	creds, ok := d.State.Credentials()
	if !ok {
		return cliproto.New(cliproto.ENotLoggedIn)
	}
	d.ensureRefreshLoopFromCreds(creds)
	if d.Refresh != nil {
		refreshed, err := d.Refresh.RefreshNowIfDue(ctx, time.Now())
		if errors.Is(err, ErrUnauthorized) {
			return cliproto.New(cliproto.EInvalidAPIKey)
		}
		if err != nil {
			return cliproto.New(cliproto.EServerUnreachable)
		}
		if refreshed && d.NC != nil {
			d.NC.Close()
			d.NC = nil
		}
	}
	if d.NC != nil && d.NC.IsConnected() {
		return nil
	}
	if d.NATSURL == "" {
		body, _ := json.Marshal(cliproto.AuthExchangeRequest{APIKey: creds.APIKey})
		req, err := http.NewRequestWithContext(ctx, "POST", creds.URL+"/api/v1/auth/exchange", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := d.HTTP.Do(req)
		if err != nil {
			return cliproto.New(cliproto.EServerUnreachable)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return cliproto.New(cliproto.EInvalidAPIKey)
		}
		var ex cliproto.AuthExchangeReply
		if err := json.NewDecoder(resp.Body).Decode(&ex); err != nil {
			return err
		}
		natsURL := ex.NATSURL
		if v := os.Getenv("PPZ_NATS_URL"); v != "" {
			natsURL = v
		}
		d.NATSURL = natsURL
		// Refresh persisted creds with the new JWT/seed so reconnects
		// don't depend on stale on-disk values.
		creds.NATSUserJWT = ex.NATSUserJWT
		creds.NATSUserSeed = ex.NATSUserSeed
		_ = d.State.SetLogin(*creds, creds.AccountID, creds.AccountName, keyPrefix(creds.APIKey))
		d.startRefreshLoop(ex.AccountID, ex.NATSUserJWT, ex.NATSUserSeed, ex.ExpiresAt.Unix())
	}
	// If we got here without /auth/exchange (NATSURL was already
	// known) but the refresh loop never started (e.g. fresh daemon
	// process post-restart), boot it from the persisted creds. Exp
	// is unknown — set to a near-past value so the loop fires
	// immediately and refreshes from the bearer.
	d.ensureRefreshLoopFromCreds(creds)
	// Did the daemon previously have a connection that was non-functional?
	// If so, the fresh NC we're about to build represents a recovery —
	// from the operator's perspective, the daemon "reconnected" to NATS.
	// Record that signal alongside the nats.go-provided ReconnectHandler
	// events so `ppz diag` surfaces both reconnect mechanisms.
	wasDisconnected := d.NC != nil && !d.NC.IsConnected()
	nc, err := connectNATSWithRefresh(d.NATSURL, d.Refresh, d.NATSEvents)
	if err != nil {
		return cliproto.New(cliproto.ENATSUnreachable)
	}
	if d.NC != nil {
		d.NC.Close()
	}
	d.NC = nc
	if wasDisconnected && d.NATSEvents != nil {
		d.NATSEvents.Append("reconnect", "ensureNATS rebuilt connection", time.Now())
	}
	return nil
}

func (d *Daemon) ensureRefreshLoopFromCreds(creds *Credentials) {
	if d.Refresh != nil || creds.NATSUserJWT == "" || creds.NATSUserSeed == "" {
		return
	}
	d.startRefreshLoop(creds.AccountID, creds.NATSUserJWT, creds.NATSUserSeed, time.Now().Add(-time.Minute).Unix())
}

// authHTTP sets the bearer header on a request.
func (d *Daemon) authHTTP(req *http.Request) error {
	creds, ok := d.State.Credentials()
	if !ok {
		return cliproto.New(cliproto.ENotLoggedIn)
	}
	req.Header.Set("Authorization", "Bearer "+creds.APIKey)
	return nil
}

// callServer is a thin JSON-in / JSON-out helper for daemon → server calls.
//
// Phase 4: when the daemon has a stored AccountID (set by login or `org
// switch`), we stamp it as `?org=<id>` on every call. The server's
// requireAPIKey middleware uses it to scope OAuth-bearer requests to
// the right tenant; without this, /api/v1/sources etc. would silently
// fall back to FirstOwnedOrgFor and the source would land in the
// wrong org. API-key callers also get the param, which the server
// validates against the key's org (a mismatch is an explicit error,
// preferred to silent ambiguity).
func (d *Daemon) callServer(ctx context.Context, method, path string, body any, out any) *cliproto.Error {
	creds, ok := d.State.Credentials()
	if !ok {
		return cliproto.New(cliproto.ENotLoggedIn)
	}
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}
	url := creds.URL + path
	if accountID := d.State.AccountID(); accountID != "" {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		url += sep + "org=" + accountID
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+creds.APIKey)
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := d.HTTP.Do(req)
	if err != nil {
		// Network failure isn't an auth verdict — leave the cached
		// loginCheck alone. Status will surface the cached state (or
		// probe if unobserved).
		return cliproto.New(cliproto.EServerUnreachable)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var herr cliproto.HTTPError
		if err := json.NewDecoder(resp.Body).Decode(&herr); err == nil && herr.Error.Code != "" {
			e := herr.Error
			if e.Code == cliproto.EInvalidAPIKey {
				d.State.SetLoginCheck(cliproto.LoginCheckInvalid)
			}
			return &e
		}
		return &cliproto.Error{Code: "E_HTTP", Message: resp.Status}
	}
	// 2xx: the credential proved itself, so flip the cache to ok. This
	// is the path that resets a stale "invalid" observation once the
	// user has run `ppz login` to refresh.
	d.State.SetLoginCheck(cliproto.LoginCheckOK)
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()}
	}
	return nil
}

func (d *Daemon) handleCreate(ctx context.Context, conn net.Conn, params json.RawMessage) {
	var req cliproto.CreateRequest
	if err := json.Unmarshal(params, &req); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()})
		return
	}
	if err := natsubj.ValidateHandle(req.Handle); err != nil {
		writeIPCErr(conn, cliproto.NewInvalidHandle(req.Handle))
		return
	}
	var reply cliproto.CreateSourceReply
	if e := d.callServer(ctx, "POST", "/api/v1/sources", cliproto.CreateSourceRequest{Handle: req.Handle, Kind: req.Kind}, &reply); e != nil {
		writeIPCErr(conn, e)
		return
	}
	d.State.RememberPipe(reply.Handle)
	// PTY sources don't become the daemon's "current" — the user retains
	// their existing current message source so `ppz broadcast` keeps working
	// the way they expect outside the terminal.
	if req.Kind != string(cliproto.KindPTY) {
		if err := d.State.SetCurrent(req.Session, reply.Handle); err != nil {
			writeIPCErr(conn, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
			return
		}
	}
	writeIPC(conn, cliproto.CreateReply{Handle: reply.Handle, Subject: reply.Subject})
}

// handleConnect is the daemon-side combo verb for `ppz connect <handle>`:
// idempotently ensure the source exists, then SetCurrent. Treats
// E_SOURCE_TAKEN from the server as "already exists, nothing to do" and
// proceeds to switch.
//
// Today (Phase C) the source-create path also auto-provisions the broadcast
// JetStream stream (carried over from the pre-refactor behaviour). When
// user-creatable pipes land (Phase E), the stream-creation step here may
// move into a dedicated "ensure broadcast pipe" step so source create can
// stop provisioning streams of its own.
func (d *Daemon) handleConnect(ctx context.Context, conn net.Conn, params json.RawMessage) {
	var req cliproto.ConnectRequest
	if err := json.Unmarshal(params, &req); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()})
		return
	}
	if _, ok := d.State.Credentials(); !ok {
		writeIPCErr(conn, cliproto.New(cliproto.ENotLoggedIn))
		return
	}
	if err := natsubj.ValidateHandle(req.Handle); err != nil {
		writeIPCErr(conn, cliproto.New(cliproto.EInvalidHandle))
		return
	}

	var reply cliproto.CreateSourceReply
	if e := d.callServer(ctx, "POST", "/api/v1/sources",
		cliproto.CreateSourceRequest{Handle: req.Handle}, &reply); e != nil {
		// Idempotent: source already exists is success.
		if e.Code != cliproto.ESourceTaken {
			writeIPCErr(conn, e)
			return
		}
	}
	d.State.RememberPipe(req.Handle)
	if err := d.State.SetCurrent(req.Session, req.Handle); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}
	writeIPC(conn, cliproto.ConnectReply{Handle: req.Handle})
}

// handlePipeCreate proxies `ppz pipe create` to the server. Daemon-side
// validation: handle must be known + name must pass ValidateUserPipeName.
// Server validates again and provisions the JetStream stream.
func (d *Daemon) handlePipeCreate(ctx context.Context, conn net.Conn, params json.RawMessage) {
	var req cliproto.PipeCreateRequest
	if err := json.Unmarshal(params, &req); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()})
		return
	}
	if _, ok := d.State.Credentials(); !ok {
		writeIPCErr(conn, cliproto.New(cliproto.ENotLoggedIn))
		return
	}
	if err := natsubj.ValidateHandle(req.Handle); err != nil {
		writeIPCErr(conn, cliproto.New(cliproto.EInvalidHandle))
		return
	}
	if err := natsubj.ValidateUserPipeName(req.Name); err != nil {
		// ValidateUserPipeName returns "invalid pipe name" (regex) or
		// "name is reserved" — keep the distinction so the user can see
		// which constraint they hit.
		if err.Error() == "name is reserved" {
			writeIPCErr(conn, cliproto.NewInvalidPipeReserved(req.Name))
		} else {
			writeIPCErr(conn, cliproto.NewInvalidPipeName(req.Name))
		}
		return
	}
	var reply cliproto.PipeCreateReply
	if e := d.callServer(ctx, "POST", "/api/v1/sources/"+req.Handle+"/pipes", req, &reply); e != nil {
		writeIPCErr(conn, e)
		return
	}
	writeIPC(conn, reply)
}

// handleSourceDestroy proxies `ppz source destroy HANDLE` to the server.
// On success it clears every session whose current equals the destroyed
// handle and removes it from the known-pipes cache.
func (d *Daemon) handleSourceDestroy(ctx context.Context, conn net.Conn, params json.RawMessage) {
	var req cliproto.SourceDestroyRequest
	if err := json.Unmarshal(params, &req); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()})
		return
	}
	if _, ok := d.State.Credentials(); !ok {
		writeIPCErr(conn, cliproto.New(cliproto.ENotLoggedIn))
		return
	}
	if err := natsubj.ValidateHandle(req.Handle); err != nil {
		writeIPCErr(conn, cliproto.NewInvalidHandle(req.Handle))
		return
	}
	if e := d.callServer(ctx, "DELETE", "/api/v1/sources/"+req.Handle, nil, nil); e != nil {
		writeIPCErr(conn, e)
		return
	}
	d.State.ForgetPipe(req.Handle)
	_ = d.State.ClearCurrentForHandle(req.Handle)
	writeIPC(conn, cliproto.SourceDestroyReply{Handle: req.Handle})
}

// handlePipeDestroy proxies `ppz pipe destroy` to the server.
func (d *Daemon) handlePipeDestroy(ctx context.Context, conn net.Conn, params json.RawMessage) {
	var req cliproto.PipeDestroyRequest
	if err := json.Unmarshal(params, &req); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()})
		return
	}
	if _, ok := d.State.Credentials(); !ok {
		writeIPCErr(conn, cliproto.New(cliproto.ENotLoggedIn))
		return
	}
	if err := natsubj.ValidateHandle(req.Handle); err != nil {
		writeIPCErr(conn, cliproto.New(cliproto.EInvalidHandle))
		return
	}
	if e := d.callServer(ctx, "DELETE", "/api/v1/sources/"+req.Handle+"/pipes/"+req.Name, nil, nil); e != nil {
		writeIPCErr(conn, e)
		return
	}
	writeIPC(conn, cliproto.PipeDestroyReply{Handle: req.Handle, Name: req.Name})
}

// handleDisconnect clears the calling session's current source. Source
// itself stays provisioned. Other sessions' currents are unaffected.
// Idempotent — calling on no-current is fine.
func (d *Daemon) handleDisconnect(_ context.Context, conn net.Conn, params json.RawMessage) {
	var req cliproto.DisconnectRequest
	_ = json.Unmarshal(params, &req) // optional body — empty is fine
	if _, ok := d.State.Credentials(); !ok {
		writeIPCErr(conn, cliproto.New(cliproto.ENotLoggedIn))
		return
	}
	if err := d.State.ClearCurrent(req.Session); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}
	writeIPC(conn, cliproto.DisconnectReply{})
}

func (d *Daemon) handleSwitch(ctx context.Context, conn net.Conn, params json.RawMessage) {
	var req cliproto.SwitchRequest
	if err := json.Unmarshal(params, &req); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()})
		return
	}
	if _, ok := d.State.Credentials(); !ok {
		writeIPCErr(conn, cliproto.New(cliproto.ENotLoggedIn))
		return
	}
	// Verify the handle exists server-side. Refresh the cache from /api/v1/sources.
	var lr cliproto.ListSourcesReply
	if e := d.callServer(ctx, "GET", "/api/v1/sources", nil, &lr); e != nil {
		writeIPCErr(conn, e)
		return
	}
	handles := make([]string, 0, len(lr.Sources))
	for _, s := range lr.Sources {
		handles = append(handles, s.Handle)
	}
	d.State.ResetPipes(handles)
	if !d.State.KnowsPipe(req.Handle) {
		writeIPCErr(conn, cliproto.NewSourceNotFound(req.Handle))
		return
	}
	if err := d.State.SetCurrent(req.Session, req.Handle); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}
	writeIPC(conn, cliproto.SwitchReply{Handle: req.Handle})
}

func (d *Daemon) handleBroadcast(ctx context.Context, conn net.Conn, params json.RawMessage) {
	var req cliproto.BroadcastRequest
	if err := json.Unmarshal(params, &req); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()})
		return
	}
	// Trust-boundary check (v0.25.0, spec §3): the `ack:` subject prefix is
	// reserved for daemon-emitted protocol messages. CLI argument
	// validation is belt; this is suspenders — any IPC client (custom
	// scripts, third-party tools, harness adapters) hits this same path.
	// Daemon-internal ack auto-emission (§4) bypasses handleBroadcast and
	// publishes envelopes directly, so this rule has no exception.
	if strings.HasPrefix(req.MsgSubject, "ack:") {
		writeIPCErr(conn, cliproto.New(cliproto.EInvalidSubject))
		return
	}
	target, e := d.resolveBroadcastTarget(ctx, req.Handle, req.Channel, req.Session)
	if e != nil {
		writeIPCErr(conn, e)
		return
	}
	env := buildBroadcastEnvelope(req, target.sender, clock.Now())
	data, err := env.Marshal()
	if err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}
	if len(data) > envelope.MaxBytes {
		writeIPCErr(conn, cliproto.New(cliproto.EPayloadTooLarge))
		return
	}
	if err := d.NC.Publish(target.subject, data); err != nil {
		writeIPCErr(conn, cliproto.New(cliproto.ENATSUnreachable))
		return
	}
	if err := d.NC.Flush(); err != nil {
		writeIPCErr(conn, cliproto.New(cliproto.ENATSUnreachable))
		return
	}
	// Bytes counts the user-visible payload, not the encoded envelope —
	// matches WIRE.md §8 ppz broadcast and the broadcast-from-* fixtures.
	writeIPC(conn, cliproto.BroadcastReply{ID: env.ID, Subject: target.subject, Bytes: len(req.Payload)})
}

// handleBroadcastBatch publishes N payloads in one IPC round-trip.
// Validation runs once for the whole batch; the daemon then issues N
// async nc.Publish calls followed by a SINGLE nc.Flush. Same "bytes
// confirmed at server" contract as handleBroadcast — just amortised
// across the batch. Used by streaming producers (terminal share's
// stdout drain, `ppz broadcast` line-streaming) where the per-call
// flush cost dominates throughput under WAN latency.
func (d *Daemon) handleBroadcastBatch(ctx context.Context, conn net.Conn, params json.RawMessage) {
	var req cliproto.BroadcastBatchRequest
	if err := json.Unmarshal(params, &req); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()})
		return
	}
	if len(req.Payloads) == 0 {
		writeIPC(conn, cliproto.BroadcastBatchReply{})
		return
	}
	target, e := d.resolveBroadcastTarget(ctx, req.Handle, req.Channel, req.Session)
	if e != nil {
		writeIPCErr(conn, e)
		return
	}
	ids := make([]string, 0, len(req.Payloads))
	bytes := make([]int, 0, len(req.Payloads))
	now := clock.Now()
	for _, payload := range req.Payloads {
		env := envelope.New(target.sender, "", payload, now)
		data, err := env.Marshal()
		if err != nil {
			writeIPCErr(conn, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
			return
		}
		if len(data) > envelope.MaxBytes {
			writeIPCErr(conn, cliproto.New(cliproto.EPayloadTooLarge))
			return
		}
		if err := d.NC.Publish(target.subject, data); err != nil {
			writeIPCErr(conn, cliproto.New(cliproto.ENATSUnreachable))
			return
		}
		ids = append(ids, env.ID)
		bytes = append(bytes, len(payload))
	}
	if err := d.NC.Flush(); err != nil {
		writeIPCErr(conn, cliproto.New(cliproto.ENATSUnreachable))
		return
	}
	writeIPC(conn, cliproto.BroadcastBatchReply{IDs: ids, Subject: target.subject, Bytes: bytes})
}

// broadcastTarget bundles the resolved facts a publish needs: the
// destination subject + the sender id we stamp into the envelope.
type broadcastTarget struct {
	subject string
	sender  string
}

// resolveBroadcastTarget runs the shared pre-flight for a broadcast:
// login check, target resolution (request handle, env, session
// current), pipe-name validation, server-side source existence (with
// stale-current cleanup), JetStream stream existence, and ensureNATS.
// Returns the destination subject + sender id on success. Used by
// both handleBroadcast (single) and handleBroadcastBatch (N).
func (d *Daemon) resolveBroadcastTarget(ctx context.Context, reqHandle, reqChannel, session string) (broadcastTarget, *cliproto.Error) {
	if _, ok := d.State.Credentials(); !ok {
		return broadcastTarget{}, cliproto.New(cliproto.ENotLoggedIn)
	}
	// Resolve target. Explicit handle from the request wins (used by
	// `ppz send` and by `ppz broadcast` when PPZ_CURRENT_HANDLE is
	// set); otherwise fall back to the calling session's current
	// handle on .broadcast — different terminal windows have
	// different "current"s.
	fromCurrent := reqHandle == "" && os.Getenv("PPZ_CURRENT_HANDLE") == ""
	current := reqHandle
	if current == "" {
		current = d.State.Current(session)
	}
	if current == "" {
		return broadcastTarget{}, cliproto.New(cliproto.ENoCurrentSource)
	}
	pipe := reqChannel
	if pipe == "" {
		pipe = "broadcast"
	}
	if err := natsubj.ValidatePipe(pipe); err != nil {
		return broadcastTarget{}, cliproto.New(cliproto.EInvalidPipe)
	}
	// Verify the handle is a real source in this org. NATS publish
	// silently succeeds against any subject, so without this check a
	// stale handle (wrapped terminal whose source was deleted, or a
	// session-current pointing at a server-side-deleted source) would
	// return `sent id=...` while the message vanishes.
	//
	// For implicit (session-current) targets, ALWAYS refresh from the
	// server: the daemon's KnowsPipe cache is durable across daemon
	// lifetimes; without a refresh, a stale per-session current set in
	// a previous shell can produce a confusing "source not found"
	// error pointing at a handle the user doesn't remember setting.
	needRefresh := !d.State.KnowsPipe(current) || fromCurrent
	if needRefresh {
		var lr cliproto.ListSourcesReply
		if e := d.callServer(ctx, "GET", "/api/v1/sources", nil, &lr); e != nil {
			return broadcastTarget{}, e
		}
		handles := make([]string, 0, len(lr.Sources))
		for _, s := range lr.Sources {
			handles = append(handles, s.Handle)
		}
		d.State.ResetPipes(handles)
		if !d.State.KnowsPipe(current) {
			if fromCurrent {
				_ = d.State.ClearCurrent(session)
				return broadcastTarget{}, cliproto.New(cliproto.ENoCurrentSource)
			}
			return broadcastTarget{}, cliproto.NewSourceNotFound(current)
		}
	}
	accountID, err := uuid.Parse(d.State.AccountID())
	if err != nil {
		return broadcastTarget{}, &cliproto.Error{Code: "E_INTERNAL", Message: "bad org id"}
	}
	if d.NC == nil || !d.NC.IsConnected() {
		if err := d.ensureNATS(ctx); err != nil {
			return broadcastTarget{}, cliproto.New(cliproto.ENATSUnreachable)
		}
	}
	// Verify the JetStream stream exists before publishing. NATS-core
	// publish to a subject with no consumer silently succeeds.
	//
	// Classify js.Stream() errors into the right user-facing code:
	//   - jetstream.ErrStreamNotFound → E_PIPE_NOT_FOUND: the pipe
	//     genuinely doesn't exist on this source. Names the pipe + source
	//     in the message so the user sees the actionable next step.
	//   - context.DeadlineExceeded / nats.ErrTimeout / nats.ErrConnectionClosed
	//     → E_NATS_UNREACHABLE: can't reach the broker to even check.
	//     The pipe might be perfectly valid (see the wifi-disconnect
	//     repro in MoltHub agent feedback); attributing the failure to
	//     pipe invalidity misled agents away from the real cause.
	//   - any other error → E_INVALID_PIPE catch-all. Truly unexpected.
	if js, err := jetstream.New(d.NC); err == nil {
		if _, err := js.Stream(ctx, natsubj.StreamName(accountID, current, pipe)); err != nil {
			switch {
			case errors.Is(err, jetstream.ErrStreamNotFound):
				return broadcastTarget{}, cliproto.NewPipeNotFound(pipe, current)
			case errors.Is(err, context.DeadlineExceeded),
				errors.Is(err, nats.ErrTimeout),
				errors.Is(err, nats.ErrConnectionClosed),
				errors.Is(err, nats.ErrNoServers):
				return broadcastTarget{}, cliproto.New(cliproto.ENATSUnreachable)
			default:
				return broadcastTarget{}, cliproto.New(cliproto.EInvalidPipe)
			}
		}
	}
	// Sender is the broadcaster's own current source — *not* the
	// destination. Different when the request pins an explicit dest
	// (`ppz send foo`, or PPZ_CURRENT_HANDLE inside `ppz terminal`).
	// Empty when this session has never connected.
	return broadcastTarget{
		subject: natsubj.Subject(accountID, current, pipe),
		sender:  d.State.Current(session),
	}, nil
}

func (d *Daemon) handleList(ctx context.Context, conn net.Conn, params json.RawMessage) {
	var req cliproto.ListRequest
	_ = json.Unmarshal(params, &req) // optional body

	if _, ok := d.State.Credentials(); !ok {
		writeIPCErr(conn, cliproto.New(cliproto.ENotLoggedIn))
		return
	}
	var lr cliproto.ListSourcesReply
	if e := d.callServer(ctx, "GET", "/api/v1/sources", nil, &lr); e != nil {
		writeIPCErr(conn, e)
		return
	}
	handles := make([]string, 0, len(lr.Sources))
	for _, s := range lr.Sources {
		handles = append(handles, s.Handle)
	}
	d.State.ResetPipes(handles)

	// Aggregate per-pipe info from JetStream (route B). List stream metadata
	// once, then fetch latest payload previews only for non-empty streams.
	if err := d.ensureNATS(ctx); err != nil {
		if e, ok := err.(*cliproto.Error); ok {
			writeIPCErr(conn, e)
		} else {
			writeIPCErr(conn, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		}
		return
	}
	js, err := jetstream.New(d.NC)
	if err != nil {
		writeIPCErr(conn, cliproto.New(cliproto.ENATSUnreachable))
		return
	}
	accountID, err := uuid.Parse(d.State.AccountID())
	if err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_INTERNAL", Message: "bad org id"})
		return
	}

	enriched, err := enrichSourcesWithPipeInfo(ctx, js, lr.Sources, accountID, req.Session, nil, cursorSnapshot(d.Cursors, req.Session))
	if err != nil {
		writeIPCErr(conn, cliproto.New(cliproto.ENATSUnreachable))
		return
	}

	writeIPC(conn, cliproto.ListReply{Sources: enriched})
}

// pipesForKind mirrors db.Source.Pipes() at the daemon level so we don't
// import internal/db just for this helper.
func pipesForKind(kind string) []string {
	if kind == string(cliproto.KindPTY) {
		// Sorted alphabetically so ls output is deterministic.
		return []string{"broadcast", "inbox", "stdctrl", "stdin", "stdout"}
	}
	return []string{"broadcast", "inbox"}
}

func daemonCursorKey(accountID uuid.UUID, handle, pipe string) string {
	return CursorKey(accountID.String(), handle, pipe)
}

func (d *Daemon) handleSubscribe(ctx context.Context, conn net.Conn, _ json.RawMessage) {
	// Phase 1: not used by any of the 35 tests. Return a clean error so the
	// CLI surface stays honest.
	writeIPCErr(conn, &cliproto.Error{Code: "E_NOT_IMPLEMENTED", Message: "Subscribe not implemented in Phase 1"})
	_ = ctx
	_ = fmt.Sprint
}
