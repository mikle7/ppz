package daemon

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

// TestArmWatch_RegistersWithRegistry covers the happy path: armWatch
// subscribes on d.NC, returns a live entry, and that entry is in the
// registry so a subsequent rearmAll will see it.
func TestArmWatch_RegistersWithRegistry(t *testing.T) {
	d, nc := newDaemonWithEmbeddedNATS(t)

	got := make(chan string, 4)
	entry, ipcErr := d.armWatch("TEST.arm.regs", func(m *nats.Msg) { got <- m.Subject })
	if ipcErr != nil {
		t.Fatalf("armWatch: %v", ipcErr)
	}
	if entry == nil {
		t.Fatal("armWatch returned nil entry")
	}
	if !d.Watches.contains(entry) {
		t.Fatal("entry not in d.Watches after armWatch")
	}
	if entry.sub == nil || !entry.sub.IsValid() {
		t.Fatal("entry.sub is nil or invalid")
	}

	// Live delivery sanity check.
	if err := nc.Publish("TEST.arm.regs", []byte("x")); err != nil {
		t.Fatalf("publish: %v", err)
	}
	select {
	case <-got:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("callback did not fire after publish")
	}

	d.Watches.remove(entry)
	if d.Watches.contains(entry) {
		t.Fatal("entry still in d.Watches after remove")
	}
}

// TestArmWatch_NoNATSConnReturnsError covers the precondition: a
// Daemon with d.NC == nil yields ENATSUnreachable instead of panicking
// on a nil-dereference Subscribe.
func TestArmWatch_NoNATSConnReturnsError(t *testing.T) {
	d := New(t.TempDir(), "")
	// d.NC is nil; no swap performed.
	_, ipcErr := d.armWatch("TEST.no.nc", func(*nats.Msg) {})
	if ipcErr == nil {
		t.Fatal("expected error when d.NC is nil, got nil")
	}
}

// TestSwapNC_RearmsArmedWatchOntoNewNC pins the core fix at the
// daemon level: an entry registered via armWatch survives a swapNC
// transparently. Publishing via the new NC after swap fires the
// callback; without the rearm, the callback would be silently dead.
func TestSwapNC_RearmsArmedWatchOntoNewNC(t *testing.T) {
	d, ncA := newDaemonWithEmbeddedNATS(t)
	url := ncA.ConnectedUrl()

	got := make(chan []byte, 16)
	subj := "TEST.swap.rearm"
	entry, ipcErr := d.armWatch(subj, func(m *nats.Msg) { got <- m.Data })
	if ipcErr != nil {
		t.Fatalf("armWatch: %v", ipcErr)
	}
	t.Cleanup(func() { d.Watches.remove(entry) })

	// Sanity: pre-swap publish on ncA delivers.
	if err := ncA.Publish(subj, []byte("pre-swap")); err != nil {
		t.Fatalf("publish pre-swap: %v", err)
	}
	if !waitFor(got, 500*time.Millisecond) {
		t.Fatal("pre-swap publish did not fire callback")
	}

	// Swap to a fresh NC (simulating a JWT-refresh rotation).
	ncB, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect ncB: %v", err)
	}
	t.Cleanup(func() { ncB.Close() })
	d.swapNC("test-rotation", ncB)

	// Post-swap: the entry's sub must now be on ncB. Publishing via
	// ncB fires the callback. Without rearm, the entry's sub would
	// be dead (anchored to the now-closed ncA) and the wakeup would
	// never fire — that's the silent-loss bug.
	if err := ncB.Publish(subj, []byte("post-swap")); err != nil {
		t.Fatalf("publish post-swap: %v", err)
	}
	if !waitFor(got, 1*time.Second) {
		t.Fatal("post-swap publish did not fire callback — rearm did not take effect")
	}
}

// TestSwapNC_RearmsArmedWatch_AcrossMultipleSwaps stresses the rearm
// path across several swaps: each swap re-Subscribes on the new conn,
// so a publish after the final swap still fires the callback.
func TestSwapNC_RearmsArmedWatch_AcrossMultipleSwaps(t *testing.T) {
	d, ncA := newDaemonWithEmbeddedNATS(t)
	url := ncA.ConnectedUrl()

	got := make(chan []byte, 32)
	subj := "TEST.swap.multi"
	entry, ipcErr := d.armWatch(subj, func(m *nats.Msg) { got <- m.Data })
	if ipcErr != nil {
		t.Fatalf("armWatch: %v", ipcErr)
	}
	t.Cleanup(func() { d.Watches.remove(entry) })

	// Three more swaps in sequence.
	var conns []*nats.Conn
	for i := 0; i < 3; i++ {
		nc, err := nats.Connect(url)
		if err != nil {
			t.Fatalf("connect swap-%d: %v", i, err)
		}
		conns = append(conns, nc)
		d.swapNC("test-rotation", nc)
	}
	finalNC := conns[len(conns)-1]
	t.Cleanup(func() { finalNC.Close() })

	if err := finalNC.Publish(subj, []byte("final")); err != nil {
		t.Fatalf("publish on final NC: %v", err)
	}
	if !waitFor(got, 1*time.Second) {
		t.Fatal("publish on final NC did not fire callback — rearm chain broken")
	}
}

// TestArmWatch_Stress_RegisterRacingWithSwap exercises the recheck
// arm of armWatch under -race: many concurrent armWatch + swap pairs
// must never leave a handler with a dead sub or trip the race
// detector. The recheck closes the window where a swap lands between
// our NC capture and our registry add().
func TestArmWatch_Stress_RegisterRacingWithSwap(t *testing.T) {
	if testing.Short() {
		t.Skip("stress test")
	}
	d, ncA := newDaemonWithEmbeddedNATS(t)
	url := ncA.ConnectedUrl()

	var wg sync.WaitGroup
	var fired int64
	subj := "TEST.stress.race"

	// Swap goroutine: continuously rotates the NC.
	stopSwap := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stopSwap:
				return
			default:
			}
			nc, err := nats.Connect(url)
			if err != nil {
				return
			}
			d.swapNC("stress", nc)
			time.Sleep(1 * time.Millisecond)
		}
	}()

	// Worker goroutines: each arms a watch, waits for a publish, exits.
	const workers = 20
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			entry, ipcErr := d.armWatch(subj, func(*nats.Msg) {
				atomic.AddInt64(&fired, 1)
			})
			if ipcErr != nil {
				return
			}
			defer d.Watches.remove(entry)
			time.Sleep(50 * time.Millisecond)
		}()
	}

	// Publisher goroutine: drives messages on the daemon's current NC
	// every few ms. Each publish should fire every currently-armed
	// callback (some workers will be mid-arm, some mid-exit — fired
	// count is non-deterministic but must be > 0 if rearm works).
	stopPub := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(2 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopPub:
				return
			case <-ticker.C:
				d.ncMu.Lock()
				nc := d.NC
				d.ncMu.Unlock()
				if nc != nil {
					_ = nc.Publish(subj, []byte("x"))
				}
			}
		}
	}()

	// Let it run.
	time.Sleep(200 * time.Millisecond)
	close(stopPub)
	close(stopSwap)
	wg.Wait()

	if atomic.LoadInt64(&fired) == 0 {
		t.Fatal("no callback fired across 20 workers and continuous swaps — rearm/recheck path broken")
	}
}

// newDaemonWithEmbeddedNATS builds a minimally-wired Daemon with
// d.NC connected to an embedded NATS server. Tests that exercise
// armWatch / swapNC / rearmAll use this — it skips the full
// SetLogin/ensureNATS path because those paths aren't under test.
func newDaemonWithEmbeddedNATS(t *testing.T) (*Daemon, *nats.Conn) {
	t.Helper()
	url := startEmbeddedNATSURL(t)
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { nc.Close() })

	d := New(t.TempDir(), "")
	d.NATSURL = url
	d.swapNC("test-init", nc) // installs nc as d.NC; stamps ncExp
	return d, nc
}

// waitFor returns true if a value arrives on c within timeout.
func waitFor[T any](c <-chan T, timeout time.Duration) bool {
	select {
	case <-c:
		return true
	case <-time.After(timeout):
		return false
	}
}
