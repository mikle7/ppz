package daemon

import (
	"testing"

	"github.com/nats-io/nats.go"
)

// TestSwapNCLocked_EmitsConnectOnFirstConnect pins the `nats: connected
// (just now)` end-to-end signal: the daemon must record a "connect"
// event whenever it transitions from no NC to a healthy NC. The CLI's
// `ppz status` reads this event's timestamp via stateSinceFrom to drive
// the "(N <unit> ago)" suffix on the `nats:` line; without it operators
// see a bare `nats: connected` with no stability signal.
//
// Regression guard for the CI break in PR #104: handleLogin's NC-
// replacement path went through swapNC directly (not rebuildNC), so the
// connect-event emission was previously living in rebuildNC alone and
// the login flow produced no anchor event. The fix moves the emission
// into swapNCLocked so every NC-changing path picks it up.
func TestSwapNCLocked_EmitsConnectOnFirstConnect(t *testing.T) {
	url := startEmbeddedNATSURL(t)
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { nc.Close() })

	d := &Daemon{
		State:      NewState(t.TempDir()),
		NATSEvents: newNATSEventRing(natsEventRingCap),
		Follows:    newFollowRegistry(),
	}
	d.swapNC("handleLogin", nc) // simulate first login: d.NC was nil

	events := d.NATSEvents.Snapshot()
	if !containsEventType(events, "connect") {
		t.Fatalf("expected a 'connect' event after first NC install; got types=%v", eventTypes(events))
	}
}

// TestSwapNCLocked_NoTransitionEventForLogout covers the watcher path:
// when credentials disappear and the watcher calls swapNC(nil) to drop
// the NC, no "connect"/"reconnect" should fire — those events anchor
// the *connected* state, not the loss of one. (A "swap" event still
// records the pointer transition for burst-swap-storm detection.)
func TestSwapNCLocked_NoTransitionEventForLogout(t *testing.T) {
	url := startEmbeddedNATSURL(t)
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	d := &Daemon{
		State:      NewState(t.TempDir()),
		NATSEvents: newNATSEventRing(natsEventRingCap),
		Follows:    newFollowRegistry(),
	}
	d.swapNC("handleLogin", nc)              // first connect → "connect"
	d.swapNC("watchState-creds-gone", nil)   // logout → no anchor event

	events := d.NATSEvents.Snapshot()
	connects, reconnects := 0, 0
	for _, e := range events {
		if e.Type == "connect" {
			connects++
		}
		if e.Type == "reconnect" {
			reconnects++
		}
	}
	if connects != 1 {
		t.Errorf("expected exactly 1 connect event (from first install); got %d (types=%v)", connects, eventTypes(events))
	}
	if reconnects != 0 {
		t.Errorf("logout (swap to nil) must not emit reconnect; got %d", reconnects)
	}
}

// TestSwapNCLocked_NoTransitionEventForRoutineRefreshSwap covers the
// JWT-rotation hot path: when the refresh loop swaps a healthy NC for a
// fresh one carrying the rotated JWT, the connection itself never went
// down, so neither "connect" nor "reconnect" should fire. Without this
// the state-since reading would jump to "now" on every rotation and
// the `nats:` line would falsely advertise a fresh recovery.
func TestSwapNCLocked_NoTransitionEventForRoutineRefreshSwap(t *testing.T) {
	url := startEmbeddedNATSURL(t)
	nc1, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect nc1: %v", err)
	}
	nc2, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect nc2: %v", err)
	}
	t.Cleanup(func() { nc2.Close() })

	d := &Daemon{
		State:      NewState(t.TempDir()),
		NATSEvents: newNATSEventRing(natsEventRingCap),
		Follows:    newFollowRegistry(),
	}
	d.swapNC("handleLogin", nc1)            // first connect
	d.swapNC("OnRefreshed-callback", nc2)   // routine refresh swap (healthy → healthy)

	events := d.NATSEvents.Snapshot()
	connects, reconnects := 0, 0
	for _, e := range events {
		if e.Type == "connect" {
			connects++
		}
		if e.Type == "reconnect" {
			reconnects++
		}
	}
	if connects != 1 {
		t.Errorf("only the first install should emit connect; got %d (types=%v)", connects, eventTypes(events))
	}
	if reconnects != 0 {
		t.Errorf("routine refresh swap (healthy → healthy) must not emit reconnect; got %d", reconnects)
	}
}

func containsEventType(events []NATSEvent, typ string) bool {
	for _, e := range events {
		if e.Type == typ {
			return true
		}
	}
	return false
}

func eventTypes(events []NATSEvent) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.Type
	}
	return out
}
