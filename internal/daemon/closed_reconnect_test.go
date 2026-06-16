package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

// TestActiveConnectionClose_TriggersBackgroundReconnect — RED for the
// self-heal gap visible in the 2026-06-15 diagnostics. When the server
// kicked the live connection (TCP reset → "nats: Authorization
// Violation" close) while the daemon was idle, nats.go's ClosedHandler
// only RECORDED the close — nothing rebuilt the connection until the
// next scheduled JWT rotation (OnRefreshed), which is up to ~4.5 min
// away when awake and far longer across sleep. reportNATSFailure →
// kickReconnect only fires from the publish/read/wait paths, i.e. when
// a command is actively running; a purely background close had no
// recovery trigger at all.
//
// Contract: a terminal close of the daemon's ACTIVE connection must
// itself kick the background reconnect — no IPC command, and without
// waiting for a refresh.
func TestActiveConnectionClose_TriggersBackgroundReconnect(t *testing.T) {
	url := startEmbeddedNATSURL(t)
	d := &Daemon{
		State:      NewState(t.TempDir()),
		NATSEvents: newNATSEventRing(natsEventRingCap),
		Follows:    newFollowRegistry(),
		NATSURL:    url,
		// Wire the REAL observe handlers so ClosedHandler fires into
		// recordNATSEvent exactly as a production connection does — the
		// seam under test is that path, not a synthetic event.
		dial: func(u string, _ *RefreshLoop, store func(NATSEvent)) (*nats.Conn, error) {
			return nats.Connect(u, natsObserveOptions(store, nil)...)
		},
	}
	loginForWakeTests(t, d)
	d.Refresh = &RefreshLoop{
		AccountID: "00000000-0000-0000-0000-000000000001",
		Refresh: func(context.Context, string) (string, string, int64, error) {
			return "jwt", "seed", time.Now().Add(5 * time.Minute).Unix(), nil
		},
	}
	if err := d.Refresh.Start(context.Background(), "jwt", "seed", time.Now().Add(5*time.Minute).Unix()); err != nil {
		t.Fatalf("RefreshLoop.Start: %v", err)
	}
	t.Cleanup(d.Refresh.Stop)

	// Establish the initial connection (the path a command would take).
	if err := d.rebuildNC("test-initial"); err != nil {
		t.Fatalf("initial rebuildNC: %v", err)
	}
	if !waitNCConnected(d, 3*time.Second) {
		t.Fatalf("initial connection never came up")
	}
	d.ncMu.Lock()
	first := d.NC
	d.ncMu.Unlock()

	// Simulate the server kicking the live connection: close it from
	// under the daemon. nats.go fires ClosedHandler — an explicit close
	// is terminal and does NOT auto-reconnect, mirroring the
	// auth-violation close. We deliberately do NOT call reportNATSFailure
	// or any IPC command: the close itself must drive recovery.
	first.Close()

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		d.ncMu.Lock()
		nc := d.NC
		d.ncMu.Unlock()
		if nc != nil && nc != first && nc.IsConnected() {
			t.Cleanup(func() { nc.Close() })
			return // GREEN: reconnected to a fresh conn on its own
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("active-connection close did not trigger a background reconnect")
}

// TestSwapClose_DoesNotChurnReconnect — guard against the obvious
// over-correction: the routine JWT-rotation swap closes the OLD
// connection (a "closed" event for an already-replaced NC). That close
// must NOT be treated as a failure — the daemon is healthy on the new
// conn, so no reconnect churn should fire and the connection must stay
// up and stable.
func TestSwapClose_DoesNotChurnReconnect(t *testing.T) {
	url := startEmbeddedNATSURL(t)
	d := &Daemon{
		State:      NewState(t.TempDir()),
		NATSEvents: newNATSEventRing(natsEventRingCap),
		Follows:    newFollowRegistry(),
		NATSURL:    url,
		dial: func(u string, _ *RefreshLoop, store func(NATSEvent)) (*nats.Conn, error) {
			return nats.Connect(u, natsObserveOptions(store, nil)...)
		},
	}
	loginForWakeTests(t, d)
	d.Refresh = &RefreshLoop{
		AccountID: "00000000-0000-0000-0000-000000000001",
		Refresh: func(context.Context, string) (string, string, int64, error) {
			return "jwt", "seed", time.Now().Add(5 * time.Minute).Unix(), nil
		},
	}
	if err := d.Refresh.Start(context.Background(), "jwt", "seed", time.Now().Add(5*time.Minute).Unix()); err != nil {
		t.Fatalf("RefreshLoop.Start: %v", err)
	}
	t.Cleanup(d.Refresh.Stop)

	if err := d.rebuildNC("test-initial"); err != nil {
		t.Fatalf("initial rebuildNC: %v", err)
	}
	if !waitNCConnected(d, 3*time.Second) {
		t.Fatalf("initial connection never came up")
	}

	// Force a swap to a brand-new connection. swapNCLocked installs the
	// new conn, then closes the old one — which fires a "closed" event
	// for the old NC. That must be recognised as an expected close.
	newNC, err := d.dial(url, d.Refresh, d.recordNATSEvent)
	if err != nil {
		t.Fatalf("dial replacement: %v", err)
	}
	d.swapNC("test-swap", newNC)

	// Give any (incorrect) churn a chance to manifest, then assert the
	// connection is stable and is exactly the one we swapped in — a
	// spurious reconnect would have replaced it again.
	time.Sleep(500 * time.Millisecond)
	d.ncMu.Lock()
	cur := d.NC
	connected := cur != nil && cur.IsConnected()
	d.ncMu.Unlock()
	if !connected {
		t.Fatalf("connection not healthy after a routine swap close")
	}
	if cur != newNC {
		t.Fatalf("routine swap-close churned a reconnect: NC replaced unexpectedly")
	}
	t.Cleanup(func() { newNC.Close() })
}
