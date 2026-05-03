package natsauth

import (
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

// startTestNATS launches an embedded NATS server with the given
// Account JWT preloaded into its in-memory resolver, and the Account's
// Operator key trusted. Returns the connection URL + a shutdown func.
//
// Implementation lives in natsauth.StartEmbeddedNATSWithAuth (stub for
// now); test-only wrapper here for ergonomic Setup.
func startTestNATS(t *testing.T, acc *Account, opJWT string) (url string, stop func()) {
	t.Helper()

	ns, err := StartEmbeddedNATSWithAuth(EmbeddedConfig{
		OperatorJWT: opJWT,
		AccountJWT:  acc.AccountJWT,
	})
	if err != nil {
		t.Fatalf("StartEmbeddedNATSWithAuth: %v", err)
	}
	if !ns.ReadyForConnections(5 * time.Second) {
		ns.Shutdown()
		t.Fatalf("NATS not ready")
	}
	return ns.ClientURL(), func() { ns.Shutdown() }
}

// TestEmbeddedNATS_RejectsTokenAuth — a daemon attempting to connect
// without a valid User JWT must be rejected. Mirrors the behaviour
// pre-Phase 3 daemons would exhibit if they tried to connect to the
// new auth-enforced server with no credentials.
func TestEmbeddedNATS_RejectsTokenAuth(t *testing.T) {
	acc, opJWT := newTestAccount(t)
	url, stop := startTestNATS(t, acc, opJWT)
	defer stop()

	_, err := nats.Connect(url, nats.Timeout(2*time.Second))
	if err == nil {
		t.Fatalf("expected nats.Connect to fail without credentials")
	}
	if !strings.Contains(err.Error(), "Authorization") &&
		!strings.Contains(err.Error(), "authorization") {
		t.Errorf("expected authorization-related error, got: %v", err)
	}
}

// TestEmbeddedNATS_AcceptsValidJWT — connecting with a valid User JWT
// minted by the Account succeeds.
func TestEmbeddedNATS_AcceptsValidJWT(t *testing.T) {
	acc, opJWT := newTestAccount(t)
	url, stop := startTestNATS(t, acc, opJWT)
	defer stop()

	jwt, seed, err := acc.MintUserJWT("alpha-uuid", 5*time.Minute)
	if err != nil {
		t.Fatalf("MintUserJWT: %v", err)
	}

	nc, err := nats.Connect(url,
		nats.UserJWTAndSeed(jwt, seed),
		nats.Timeout(2*time.Second),
	)
	if err != nil {
		t.Fatalf("nats.Connect with valid JWT: %v", err)
	}
	defer nc.Close()

	// Basic health: publish to our own org's subject + receive it back.
	sub, err := nc.SubscribeSync("alpha-uuid.foo.broadcast")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := nc.Publish("alpha-uuid.foo.broadcast", []byte("hi")); err != nil {
		t.Fatalf("publish: %v", err)
	}
	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("NextMsg: %v", err)
	}
	if string(msg.Data) != "hi" {
		t.Errorf("got %q want %q", msg.Data, "hi")
	}
}

// TestEmbeddedNATS_DenyForeignOrgPublish — a User JWT scoped to org
// alpha cannot publish (or subscribe) to a beta-prefixed subject.
// Server-side permission enforcement, not just claim inspection.
func TestEmbeddedNATS_DenyForeignOrgPublish(t *testing.T) {
	acc, opJWT := newTestAccount(t)
	url, stop := startTestNATS(t, acc, opJWT)
	defer stop()

	jwt, seed, err := acc.MintUserJWT("alpha-uuid", 5*time.Minute)
	if err != nil {
		t.Fatalf("MintUserJWT: %v", err)
	}

	// Capture async permission-violation errors via the error handler.
	errCh := make(chan error, 1)
	nc, err := nats.Connect(url,
		nats.UserJWTAndSeed(jwt, seed),
		nats.Timeout(2*time.Second),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, e error) {
			select {
			case errCh <- e:
			default:
			}
		}),
	)
	if err != nil {
		t.Fatalf("nats.Connect: %v", err)
	}
	defer nc.Close()

	// Publish to a foreign org's subject — the server must reject this
	// as a permissions violation (asynchronously delivered to the
	// error handler, since publish is fire-and-forget).
	if err := nc.Publish("beta-uuid.foo.broadcast", []byte("intruder")); err != nil {
		// Some clients raise synchronously; either is acceptable.
		if !strings.Contains(err.Error(), "Permissions") {
			t.Fatalf("publish: unexpected non-permissions error: %v", err)
		}
		return
	}
	_ = nc.Flush()
	select {
	case e := <-errCh:
		if !strings.Contains(e.Error(), "Permissions") {
			t.Errorf("expected permissions violation, got: %v", e)
		}
	case <-time.After(2 * time.Second):
		t.Errorf("expected permissions-violation error within 2s; none arrived")
	}
}

