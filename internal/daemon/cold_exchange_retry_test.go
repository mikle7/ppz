package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// TestEnsureNATS_ColdExchangeTransientStatusRetryable — RED for the
// review finding on PR #128: a freshly restarted daemon has an empty
// in-memory NATSURL, so ensureNATS always takes the cold
// /auth/exchange path. That path collapsed EVERY non-200 to
// EInvalidAPIKey — including 503/500/429 from a briefly down/overloaded
// server. kickConnect treats EInvalidAPIKey as terminal, so a server
// hiccup during the restart window (ppz upgrade / reboot) made
// startup-connect give up permanently — the exact bug this PR fixes,
// narrowed to "server unhappy at restart". A transient HTTP status must
// map to a retryable code.
func TestEnsureNATS_ColdExchangeTransientStatusRetryable(t *testing.T) {
	for _, status := range []int{
		http.StatusServiceUnavailable, // 503
		http.StatusInternalServerError, // 500
		http.StatusTooManyRequests,     // 429
		http.StatusBadGateway,          // 502
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		// Credentials WITHOUT nats jwt/seed → the refresh loop never
		// starts (ensureRefreshLoopFromCreds is a no-op), so ensureNATS
		// falls straight through to the cold exchange path under test.
		d := &Daemon{
			State:      NewState(t.TempDir()),
			NATSEvents: newNATSEventRing(natsEventRingCap),
			HTTP:       &http.Client{Timeout: 2 * time.Second},
		}
		creds := Credentials{URL: srv.URL, APIKey: "pz_test", AccountID: "00000000-0000-0000-0000-000000000001"}
		if err := d.State.SetLogin(creds, creds.AccountID, "alpha", "pz_test"); err != nil {
			t.Fatalf("SetLogin: %v", err)
		}

		err := d.ensureNATS(context.Background())
		srv.Close()

		var ce *cliproto.Error
		if !errors.As(err, &ce) {
			t.Fatalf("status %d: ensureNATS returned %v, want a *cliproto.Error", status, err)
		}
		if ce.Code != cliproto.EServerUnreachable {
			t.Fatalf("status %d: cold exchange mapped to %s, want EServerUnreachable "+
				"(a terminal code makes kickConnect give up forever)", status, ce.Code)
		}
	}
}

// TestEnsureNATS_ColdExchangeAuthStatusTerminal — guard: a genuine auth
// failure (401/403) must stay terminal so a revoked/invalid key is not
// retried forever. This held before the fix and must keep holding.
func TestEnsureNATS_ColdExchangeAuthStatusTerminal(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		d := &Daemon{
			State:      NewState(t.TempDir()),
			NATSEvents: newNATSEventRing(natsEventRingCap),
			HTTP:       &http.Client{Timeout: 2 * time.Second},
		}
		creds := Credentials{URL: srv.URL, APIKey: "pz_test", AccountID: "00000000-0000-0000-0000-000000000001"}
		if err := d.State.SetLogin(creds, creds.AccountID, "alpha", "pz_test"); err != nil {
			t.Fatalf("SetLogin: %v", err)
		}

		err := d.ensureNATS(context.Background())
		srv.Close()

		var ce *cliproto.Error
		if !errors.As(err, &ce) || ce.Code != cliproto.EInvalidAPIKey {
			t.Fatalf("status %d: cold exchange returned %v, want EInvalidAPIKey (terminal)", status, err)
		}
	}
}

// TestKickConnect_RecoversAfterTransientColdExchangeFailure — the
// end-to-end reproduction: the server returns 503 for the first two
// /auth/exchange calls (the restart-window hiccup) then succeeds.
// kickConnect must retry through the failures and connect, NOT give up
// on the first one.
func TestKickConnect_RecoversAfterTransientColdExchangeFailure(t *testing.T) {
	natsURL := startEmbeddedNATSURL(t)
	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt64(&calls, 1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cliproto.AuthExchangeReply{
			NATSURL:      natsURL,
			AccountID:    "00000000-0000-0000-0000-000000000001",
			AccountName:  "alpha",
			NATSUserJWT:  "jwt",
			NATSUserSeed: "seed",
			ExpiresAt:    time.Now().Add(5 * time.Minute),
		})
	}))
	defer srv.Close()

	d := &Daemon{
		State:            NewState(t.TempDir()),
		NATSEvents:       newNATSEventRing(natsEventRingCap),
		Follows:          newFollowRegistry(),
		Watches:          newWatchRegistry(),
		Heartbeats:       NewHeartbeatCache(),
		HTTP:             &http.Client{Timeout: 2 * time.Second},
		reconnectBackoff: 5 * time.Millisecond,
		// Stub dial so the no-auth embedded server accepts us regardless
		// of the placeholder jwt/seed — the path under test is the cold
		// exchange + retry, not NATS auth.
		dial: func(u string, _ *RefreshLoop, store func(NATSEvent)) (*nats.Conn, error) {
			return nats.Connect(u, natsObserveOptions(store, nil)...)
		},
	}
	creds := Credentials{URL: srv.URL, APIKey: "pz_test", AccountID: "00000000-0000-0000-0000-000000000001"}
	if err := d.State.SetLogin(creds, creds.AccountID, "alpha", "pz_test"); err != nil {
		t.Fatalf("SetLogin: %v", err)
	}

	d.kickConnect(context.Background(), "test")

	if !waitNCConnected(d, 5*time.Second) {
		t.Fatalf("kickConnect gave up on transient cold-exchange failures instead of retrying to success")
	}
	t.Cleanup(func() {
		if d.Refresh != nil {
			d.Refresh.Stop()
		}
		if d.NC != nil {
			d.NC.Close()
		}
	})
	if got := atomic.LoadInt64(&calls); got < 3 {
		t.Fatalf("only %d exchange attempts; expected retries through the 503s", got)
	}
}

// TestEnsureNATS_ConcurrentColdStartSingleExchange — RED for the
// cold-path double-entry race amplified by PR #128. connectOnStartup's
// background ensureNATS and the first IPC command's ensureNATS can both
// observe NATSURL=="" and run the cold /auth/exchange block
// concurrently, racing the unsynchronized writes to d.NATSURL and
// d.Refresh (startRefreshLoop runs twice → orphaned refresh goroutine).
// The cold bootstrap must be single-flighted: exactly one exchange, one
// refresh loop, regardless of how many callers race in.
//
// The server-side delay widens the check-then-set window so the race is
// reliably exercised; -race on this test also covers the field writes.
func TestEnsureNATS_ConcurrentColdStartSingleExchange(t *testing.T) {
	natsURL := startEmbeddedNATSURL(t)
	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&calls, 1)
		time.Sleep(40 * time.Millisecond) // widen the check-then-set window
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cliproto.AuthExchangeReply{
			NATSURL:      natsURL,
			AccountID:    "00000000-0000-0000-0000-000000000001",
			AccountName:  "alpha",
			NATSUserJWT:  "jwt",
			NATSUserSeed: "seed",
			ExpiresAt:    time.Now().Add(5 * time.Minute),
		})
	}))
	defer srv.Close()

	d := &Daemon{
		State:      NewState(t.TempDir()),
		NATSEvents: newNATSEventRing(natsEventRingCap),
		Follows:    newFollowRegistry(),
		Watches:    newWatchRegistry(),
		Heartbeats: NewHeartbeatCache(),
		HTTP:       &http.Client{Timeout: 2 * time.Second},
		dial: func(u string, _ *RefreshLoop, store func(NATSEvent)) (*nats.Conn, error) {
			return nats.Connect(u, natsObserveOptions(store, nil)...)
		},
	}
	creds := Credentials{URL: srv.URL, APIKey: "pz_test", AccountID: "00000000-0000-0000-0000-000000000001"}
	if err := d.State.SetLogin(creds, creds.AccountID, "alpha", "pz_test"); err != nil {
		t.Fatalf("SetLogin: %v", err)
	}

	const goroutines = 8
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = d.ensureNATS(context.Background())
		}()
	}
	wg.Wait()
	t.Cleanup(func() {
		if d.Refresh != nil {
			d.Refresh.Stop()
		}
		if d.NC != nil {
			d.NC.Close()
		}
	})

	if got := atomic.LoadInt64(&calls); got != 1 {
		t.Fatalf("/auth/exchange called %d times for %d concurrent cold-start callers, want 1 "+
			"(unsynchronized cold bootstrap double-enters)", got, goroutines)
	}
	if !waitNCConnected(d, 3*time.Second) {
		t.Fatalf("concurrent cold start did not converge to a connection")
	}
}
