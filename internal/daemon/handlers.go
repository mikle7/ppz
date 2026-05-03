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
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nkeys"

	"github.com/pipescloud/ppz/internal/clock"
	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/envelope"
	"github.com/pipescloud/ppz/internal/natsubj"
)

// connectNATSWithJWT connects to NATS at url, authenticating with the
// supplied User JWT + seed (Phase 3). Static — kept for legacy code
// paths that don't yet flow through the refresh loop.
func connectNATSWithJWT(url, userJWT, userSeed string) (*nats.Conn, error) {
	if userJWT == "" || userSeed == "" {
		return nil, errors.New("connectNATSWithJWT: missing nats user jwt/seed in credentials")
	}
	kp, err := nkeys.FromSeed([]byte(userSeed))
	if err != nil {
		return nil, fmt.Errorf("parse user seed: %w", err)
	}
	return nats.Connect(url,
		nats.UserJWT(
			func() (string, error) { return userJWT, nil },
			func(nonce []byte) ([]byte, error) { return kp.Sign(nonce) },
		),
	)
}

// connectNATSWithRefresh connects to NATS at url, reading the live
// User JWT + seed from the supplied RefreshLoop on every (re)connect.
// nats.go calls the callbacks once per connection establishment; if
// the refresh loop has rotated credentials in the meantime, the next
// reconnect picks up the fresh values.
func connectNATSWithRefresh(url string, r *RefreshLoop) (*nats.Conn, error) {
	jwt, seed := r.Current()
	if jwt == "" || seed == "" {
		return nil, errors.New("connectNATSWithRefresh: refresh loop has no credentials yet")
	}
	return nats.Connect(url,
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
	)
}

// authExchangeRefresh is the RefreshFn we register with RefreshLoop.
// On every fire it re-runs POST /api/v1/auth/exchange with the
// daemon's bearer for the supplied orgID, and returns the new
// (jwt, seed, exp). 401 → ErrUnauthorized so the loop stops + the
// daemon's loginCheck flips to invalid.
func (d *Daemon) authExchangeRefresh(ctx context.Context, orgID string) (string, string, int64, error) {
	creds, ok := d.State.Credentials()
	if !ok {
		return "", "", 0, ErrUnauthorized
	}
	body, _ := json.Marshal(cliproto.AuthExchangeRequest{APIKey: creds.APIKey})
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
	_ = d.State.SetLogin(*creds, ex.OrgID, ex.OrgName, keyPrefix(creds.APIKey))
	return ex.NATSUserJWT, ex.NATSUserSeed, ex.ExpiresAt.Unix(), nil
}

// startRefreshLoop swaps d.Refresh for a fresh RefreshLoop tied to
// the supplied initial credentials. Idempotent — stops any existing
// loop first.
func (d *Daemon) startRefreshLoop(orgID, jwt, seed string, expUnix int64) {
	if d.Refresh != nil {
		d.Refresh.Stop()
	}
	d.Refresh = &RefreshLoop{
		OrgID:   orgID,
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
	if err := d.State.SetLogin(creds, ex.OrgID, ex.OrgName, prefix); err != nil {
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
	d.startRefreshLoop(ex.OrgID, ex.NATSUserJWT, ex.NATSUserSeed, ex.ExpiresAt.Unix())
	if nc, err := connectNATSWithRefresh(natsURL, d.Refresh); err == nil {
		d.NC = nc
	}

	writeIPC(conn, cliproto.LoginReply{URL: req.URL, KeyPrefix: prefix, OrgID: ex.OrgID})
}

// ensureNATS establishes the daemon's NATS connection from stored
// credentials. Login does this proactively, but a daemon restarted via
// `ppz kill` + `ppz daemon` only reloads creds from disk — d.NATSURL is
// in-memory state that doesn't survive. We rebuild it here on demand by
// re-calling /auth/exchange.
func (d *Daemon) ensureNATS(ctx context.Context) error {
	if d.NC != nil && d.NC.IsConnected() {
		return nil
	}
	creds, ok := d.State.Credentials()
	if !ok {
		return cliproto.New(cliproto.ENotLoggedIn)
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
		_ = d.State.SetLogin(*creds, creds.OrgID, creds.OrgName, keyPrefix(creds.APIKey))
		d.startRefreshLoop(ex.OrgID, ex.NATSUserJWT, ex.NATSUserSeed, ex.ExpiresAt.Unix())
	}
	// If we got here without /auth/exchange (NATSURL was already
	// known) but the refresh loop never started (e.g. fresh daemon
	// process post-restart), boot it from the persisted creds. Exp
	// is unknown — set to a near-past value so the loop fires
	// immediately and refreshes from the bearer.
	if d.Refresh == nil {
		d.startRefreshLoop(creds.OrgID, creds.NATSUserJWT, creds.NATSUserSeed, time.Now().Add(-time.Minute).Unix())
	}
	nc, err := connectNATSWithRefresh(d.NATSURL, d.Refresh)
	if err != nil {
		return cliproto.New(cliproto.ENATSUnreachable)
	}
	if d.NC != nil {
		d.NC.Close()
	}
	d.NC = nc
	return nil
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
	req, err := http.NewRequestWithContext(ctx, method, creds.URL+path, bodyReader)
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
	if _, ok := d.State.Credentials(); !ok {
		writeIPCErr(conn, cliproto.New(cliproto.ENotLoggedIn))
		return
	}

	// Resolve target. Explicit handle/channel from the request wins (used
	// by `ppz send` and by `ppz broadcast` when PPZ_CURRENT_HANDLE is set);
	// otherwise fall back to the calling session's current handle on
	// .broadcast — different terminal windows have different "current"s.
	fromCurrent := req.Handle == "" && os.Getenv("PPZ_CURRENT_HANDLE") == ""
	current := req.Handle
	if current == "" {
		current = d.State.Current(req.Session)
	}
	if current == "" {
		writeIPCErr(conn, cliproto.New(cliproto.ENoCurrentSource))
		return
	}
	pipe := req.Channel
	if pipe == "" {
		pipe = "broadcast"
	}
	if err := natsubj.ValidatePipe(pipe); err != nil {
		writeIPCErr(conn, cliproto.New(cliproto.EInvalidPipe))
		return
	}
	// Verify the resolved handle is a real source in this org. NATS
	// publish silently succeeds against any subject, so without this
	// check a stale handle (wrapped terminal whose source was deleted,
	// or a session-current pointing at a server-side-deleted source)
	// would return `sent id=...` while the message vanishes.
	//
	// For implicit (session-current) targets, ALWAYS refresh from the
	// server. The daemon's KnowsPipe cache is durable across daemon
	// lifetimes; without a refresh, a stale per-session current set in
	// a previous shell can produce a confusing "source not found" error
	// pointing at a handle the user doesn't remember setting. Detecting
	// the staleness here lets us clear it transparently and surface the
	// actionable error instead.
	needRefresh := !d.State.KnowsPipe(current) || fromCurrent
	if needRefresh {
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
		if !d.State.KnowsPipe(current) {
			if fromCurrent {
				// Stale per-session current — clear it so the next call
				// starts clean, and tell the user there's no current
				// (the actionable error: "run `ppz source create HANDLE`").
				_ = d.State.ClearCurrent(req.Session)
				writeIPCErr(conn, cliproto.New(cliproto.ENoCurrentSource))
				return
			}
			writeIPCErr(conn, cliproto.NewSourceNotFound(current))
			return
		}
	}
	orgIDStr := d.State.OrgID()
	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_INTERNAL", Message: "bad org id"})
		return
	}
	env := envelope.New(current, req.Payload, clock.Now())
	data, err := env.Marshal()
	if err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}
	if len(data) > envelope.MaxBytes {
		writeIPCErr(conn, cliproto.New(cliproto.EPayloadTooLarge))
		return
	}
	subject := natsubj.Subject(orgID, current, pipe)

	if d.NC == nil || !d.NC.IsConnected() {
		if err := d.ensureNATS(ctx); err != nil {
			writeIPCErr(conn, cliproto.New(cliproto.ENATSUnreachable))
			return
		}
	}
	// Verify the JetStream stream exists before publishing. NATS-core publish
	// to a subject with no consumer silently succeeds; without this guard a
	// `send foo.typo` would return `sent id=...` and the message would
	// vanish. Stream lookup is a single round-trip — cheap.
	if js, err := jetstream.New(d.NC); err == nil {
		if _, err := js.Stream(ctx, natsubj.StreamName(orgID, current, pipe)); err != nil {
			writeIPCErr(conn, cliproto.New(cliproto.EInvalidPipe))
			return
		}
	}
	if err := d.NC.Publish(subject, data); err != nil {
		writeIPCErr(conn, cliproto.New(cliproto.ENATSUnreachable))
		return
	}
	if err := d.NC.Flush(); err != nil {
		writeIPCErr(conn, cliproto.New(cliproto.ENATSUnreachable))
		return
	}
	// Bytes counts the user-visible payload, not the encoded envelope —
	// matches WIRE.md §8 ppz broadcast and the broadcast-from-* fixtures.
	writeIPC(conn, cliproto.BroadcastReply{ID: env.ID, Subject: subject, Bytes: len(req.Payload)})
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

	// Aggregate per-pipe info from JetStream (route B). For each source
	// we look at the pipe set implied by its kind, ask JetStream for
	// each stream's Info, and join with the session's cursor.
	if err := d.ensureNATS(ctx); err != nil {
		// No NATS = can't get per-pipe stats. Return what we have.
		writeIPC(conn, cliproto.ListReply{Sources: lr.Sources})
		return
	}
	js, err := jetstream.New(d.NC)
	if err != nil {
		writeIPC(conn, cliproto.ListReply{Sources: lr.Sources})
		return
	}
	orgID, err := uuid.Parse(d.State.OrgID())
	if err != nil {
		writeIPC(conn, cliproto.ListReply{Sources: lr.Sources})
		return
	}

	enriched := make([]cliproto.Source, 0, len(lr.Sources))
	for _, s := range lr.Sources {
		// Combine auto-provisioned pipes (from kind) with user-created
		// pipes (returned by the server). Dedupe in case a user explicitly
		// created an auto-provisioned name on a pty source — the row +
		// stream coexist; ls should show one line.
		pipeSet := map[string]struct{}{}
		for _, p := range pipesForKind(s.Kind) {
			pipeSet[p] = struct{}{}
		}
		for _, p := range s.Pipes {
			pipeSet[p] = struct{}{}
		}
		pipes := make([]string, 0, len(pipeSet))
		for p := range pipeSet {
			pipes = append(pipes, p)
		}
		sort.Strings(pipes)
		infos := make([]cliproto.PipeInfo, 0, len(pipes))
		for _, p := range pipes {
			info := cliproto.PipeInfo{Pipe: p}
			streamName := natsubj.StreamName(orgID, s.Handle, p)
			if stream, err := js.Stream(ctx, streamName); err == nil {
				if si, err := stream.Info(ctx); err == nil {
					info.Total = si.State.Msgs
					info.LastSeq = si.State.LastSeq
					if !si.State.LastTime.IsZero() {
						lt := si.State.LastTime.UTC()
						info.LastAt = &lt
					}
					cursor := d.Cursors.Get(req.Session, daemonCursorKey(orgID, s.Handle, p))
					if info.LastSeq > cursor {
						info.Unread = info.LastSeq - cursor
					}
					// Preview = TruncatePayload of the most recent
					// envelope's payload field. Works uniformly for
					// broadcast / stdin / stdout pipes — the postgres
					// last_broadcast_payload mirror is purely for the
					// server GUI now.
					if info.LastSeq > 0 {
						if msg, err := stream.GetMsg(ctx, info.LastSeq); err == nil {
							if env, err := envelope.Unmarshal(msg.Data); err == nil {
								info.Preview = cliproto.TruncatePayload(env.Payload)
								// Full payload is consumed by `ppz ls --json`.
								// Bounded by JetStream message size, so even a
								// busy-source ls call stays within reasonable
								// IPC payload bounds.
								info.Payload = env.Payload
							}
						}
					}
				}
			}
			infos = append(infos, info)
		}
		s.PipeInfos = infos
		enriched = append(enriched, s)
	}

	writeIPC(conn, cliproto.ListReply{Sources: enriched})
}

// pipesForKind mirrors db.Source.Pipes() at the daemon level so we don't
// import internal/db just for this helper.
func pipesForKind(kind string) []string {
	if kind == string(cliproto.KindPTY) {
		// Sorted alphabetically so ls output is deterministic.
		return []string{"broadcast", "stdctrl", "stdin", "stdout"}
	}
	return []string{"broadcast"}
}

func daemonCursorKey(orgID uuid.UUID, handle, pipe string) string {
	return CursorKey(orgID.String(), handle, pipe)
}

func (d *Daemon) handleSubscribe(ctx context.Context, conn net.Conn, _ json.RawMessage) {
	// Phase 1: not used by any of the 35 tests. Return a clean error so the
	// CLI surface stays honest.
	writeIPCErr(conn, &cliproto.Error{Code: "E_NOT_IMPLEMENTED", Message: "Subscribe not implemented in Phase 1"})
	_ = ctx
	_ = fmt.Sprint
}
