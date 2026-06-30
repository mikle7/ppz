package daemon

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// These tests pin the multi-org cold-restart invariant for bootstrapNATS:
// the account the daemon advertises (State.AccountID(), used to stamp
// ?org= on every HTTP list AND to build every NATS subject) MUST match
// the account the minted NATS user JWT is actually bound to. When they
// diverge, `ppz ls` lists one org's pipes over a JWT scoped to another —
// the pipes show up but read empty / can't be sent to.
//
// A cold-restarted daemon (NATSURL == "" in memory) re-runs
// /auth/exchange. Two defects broke the invariant there:
//   - the exchange request omitted AccountID, so the server fell back to
//     its default org instead of the org the user logged into; and
//   - SetLogin re-used the stale persisted creds.AccountID instead of the
//     account the server just minted the JWT in.

// newColdDaemon builds a daemon in the cold-restart shape: credentials on
// disk with an AccountID but no NATS JWT/seed, so bootstrapNATS takes the
// /auth/exchange path.
func newColdDaemon(t *testing.T, serverURL, accountID string) (*Daemon, *Credentials) {
	t.Helper()
	d := &Daemon{
		State:      NewState(t.TempDir()),
		NATSEvents: newNATSEventRing(natsEventRingCap),
		HTTP:       &http.Client{Timeout: 2 * time.Second},
	}
	creds := Credentials{URL: serverURL, APIKey: "ppz_oauth_test", AccountID: accountID}
	if err := d.State.SetLogin(creds, accountID, "beta", "ppz_oauth_"); err != nil {
		t.Fatalf("SetLogin: %v", err)
	}
	t.Cleanup(func() {
		if d.Refresh != nil {
			d.Refresh.Stop()
		}
		if d.NC != nil {
			d.NC.Close()
		}
	})
	return d, &creds
}

// TestBootstrapNATS_SendsStoredAccountID — the cold-restart exchange must
// carry the org the user logged into, otherwise the server mints the JWT
// in its own default org.
func TestBootstrapNATS_SendsStoredAccountID(t *testing.T) {
	const beta = "00000000-0000-0000-0000-0000000000b2"

	var gotReqAccountID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req cliproto.AuthExchangeRequest
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &req)
		gotReqAccountID = req.AccountID
		// A correct server mints in the requested org; echo it back.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cliproto.AuthExchangeReply{
			NATSURL:      "nats://127.0.0.1:1",
			AccountID:    req.AccountID,
			AccountName:  "beta",
			NATSUserJWT:  "jwt",
			NATSUserSeed: "seed",
			ExpiresAt:    time.Now().Add(5 * time.Minute),
		})
	}))
	defer srv.Close()

	d, creds := newColdDaemon(t, srv.URL, beta)
	if err := d.bootstrapNATS(context.Background(), creds); err != nil {
		t.Fatalf("bootstrapNATS: %v", err)
	}

	if gotReqAccountID != beta {
		t.Fatalf("cold exchange sent AccountID=%q, want %q (the org the user logged into); "+
			"omitting it makes the server mint the JWT in its default org", gotReqAccountID, beta)
	}
}

// TestBootstrapNATS_AdoptsServerAccount — whatever account the server
// reports minting the JWT in, State.AccountID() must follow it. Re-using
// the stale persisted AccountID is exactly what splits listing (?org=)
// from the NATS creds.
func TestBootstrapNATS_AdoptsServerAccount(t *testing.T) {
	const stale = "00000000-0000-0000-0000-0000000000b2" // what's on disk
	const minted = "00000000-0000-0000-0000-0000000000a1" // what the server actually minted in

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cliproto.AuthExchangeReply{
			NATSURL:      "nats://127.0.0.1:1",
			AccountID:    minted,
			AccountName:  "alpha",
			NATSUserJWT:  "jwt",
			NATSUserSeed: "seed",
			ExpiresAt:    time.Now().Add(5 * time.Minute),
		})
	}))
	defer srv.Close()

	d, creds := newColdDaemon(t, srv.URL, stale)
	if err := d.bootstrapNATS(context.Background(), creds); err != nil {
		t.Fatalf("bootstrapNATS: %v", err)
	}

	if got := d.State.AccountID(); got != minted {
		t.Fatalf("State.AccountID()=%q after cold exchange, want %q (the account the JWT was minted in); "+
			"keeping the stale on-disk AccountID desyncs ?org= listing from the NATS creds", got, minted)
	}
}
