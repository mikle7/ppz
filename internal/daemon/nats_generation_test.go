package daemon

import (
	"sync/atomic"
	"testing"

	"github.com/nats-io/nats.go"
)

// TestSwapNC_StampsNCGeneration pins that every NC swap maintains d.ncExp —
// the JWT generation the connection was dialed against. swapNC is the one
// choke point all NC replacements pass through (handleLogin, watcher,
// rebuildNC), so stamping ncExp here keeps the generation consistent
// everywhere.
//
// RED today: only rebuildNC stamps ncExp, so handleLogin's swapNC leaves
// ncExp=0 while Refresh.JWTExp() is already non-zero — the next ensureNATS
// then sees a stale generation and does a redundant reconnect+swap on a
// perfectly healthy post-login connection. Centralising the stamp in
// swapNCLocked fixes that.
func TestSwapNC_StampsNCGeneration(t *testing.T) {
	url := startEmbeddedNATSURL(t)
	d := &Daemon{
		State:      NewState(t.TempDir()),
		NATSEvents: newNATSEventRing(natsEventRingCap),
		Follows:    newFollowRegistry(),
		Refresh:    &RefreshLoop{},
	}
	d.Refresh.expUnix = 4242 // current JWT generation

	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect embedded nats: %v", err)
	}
	t.Cleanup(nc.Close)

	d.swapNC("test-login", nc)
	if d.ncExp != 4242 {
		t.Errorf("after swapNC(non-nil), ncExp = %d, want 4242 (must stamp the dialed JWT generation)", d.ncExp)
	}

	// Swapping to nil (creds gone) clears the generation — there's no live
	// connection to attribute one to.
	d.swapNC("test-clear", nil)
	if d.ncExp != 0 {
		t.Errorf("after swapNC(nil), ncExp = %d, want 0", d.ncExp)
	}
}

// TestRebuildNC_RedialsOnceOnRotation exercises the stale-generation path
// the storm fix exists for (the original test only covered cold-start with
// Refresh=nil): a healthy connection on an OLD JWT generation must be
// rebuilt exactly once when the generation bumps, then left alone.
func TestRebuildNC_RedialsOnceOnRotation(t *testing.T) {
	url := startEmbeddedNATSURL(t)
	var dials int64
	d := &Daemon{
		State:      NewState(t.TempDir()),
		NATSEvents: newNATSEventRing(natsEventRingCap),
		Follows:    newFollowRegistry(),
		Refresh:    &RefreshLoop{},
		NATSURL:    url,
		dial: func(u string, _ *RefreshLoop, _ func(NATSEvent)) (*nats.Conn, error) {
			atomic.AddInt64(&dials, 1)
			return nats.Connect(u)
		},
	}
	d.Refresh.expUnix = 1000

	// Cold start: one dial.
	if err := d.rebuildNC("ensureNATS"); err != nil {
		t.Fatalf("cold-start rebuildNC: %v", err)
	}
	// Steady state on the same generation: no-op.
	_ = d.rebuildNC("ensureNATS")
	if n := atomic.LoadInt64(&dials); n != 1 {
		t.Fatalf("steady-state dials = %d, want 1 (must not redial a current connection)", n)
	}

	// JWT rotation: generation bumps → exactly one re-dial, then quiet.
	d.Refresh.expUnix = 2000
	_ = d.rebuildNC("ensureNATS")
	_ = d.rebuildNC("ensureNATS")
	if n := atomic.LoadInt64(&dials); n != 2 {
		t.Fatalf("post-rotation dials = %d, want 2 (one re-dial per rotation, coalesced)", n)
	}
	if d.NC != nil {
		t.Cleanup(func() { d.NC.Close() })
	}
}
