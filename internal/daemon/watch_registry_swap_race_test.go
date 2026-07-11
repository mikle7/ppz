package daemon

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

// TestPublishDuringSwapWindowStillWakesWatch guards against a
// mikle7/ppz#1 hypothesis: does a message published concurrently with
// (right around) a swapNC call reliably still wake an armed watch entry
// (the mechanism behind `subs wait` / the terminal-share nudge pump)?
// Team diagnostics (herald/relay/echo/chorus) correlated real
// message-loss with the disconnect/closed window of the routine ~4m30s
// JWT-refresh swap cycle; rearmAll's dual-subscribe-then-flush-then-
// close-old design looks correct on paper (and
// TestSwapNC_RearmsArmedWatch_AcrossMultipleSwaps already proves it for
// a publish *after* a swap completes) — this test instead fires
// publishes *during* the swap window itself, repeatedly, to hunt for a
// narrower timing gap the existing tests don't exercise.
//
// RESULT (2026-07-11): 40/40 rounds observed, 0 misses — this
// mechanism is NOT the source of the reported surfacing gap. Kept as a
// permanent regression test; ruling this out narrows the search for
// whoever continues chasing #1.
func TestPublishDuringSwapWindowStillWakesWatch(t *testing.T) {
	d, ncA := newDaemonWithEmbeddedNATS(t)
	url := ncA.ConnectedUrl()

	got := make(chan []byte, 256)
	subj := "TEST.swap.race"
	entry, ipcErr := d.armWatch(subj, func(m *nats.Msg) { got <- m.Data })
	if ipcErr != nil {
		t.Fatalf("armWatch: %v", ipcErr)
	}
	t.Cleanup(func() { d.Watches.remove(entry) })

	const rounds = 40
	missed := 0
	var conns []*nats.Conn
	t.Cleanup(func() {
		for _, c := range conns {
			c.Close()
		}
	})

	for i := 0; i < rounds; i++ {
		nc, err := nats.Connect(url)
		if err != nil {
			t.Fatalf("connect round-%d: %v", i, err)
		}
		conns = append(conns, nc)

		// Fire the publish concurrently with the swap — a separate,
		// independent connection, exactly like a sibling agent's
		// `ppz send` racing this daemon's own JWT-refresh swap.
		pubDone := make(chan error, 1)
		go func(round int) {
			pubNC, err := nats.Connect(url)
			if err != nil {
				pubDone <- err
				return
			}
			defer pubNC.Close()
			payload := []byte{byte(round)}
			pubDone <- pubNC.Publish(subj, payload)
		}(i)

		d.swapNC("test-rotation-race", nc)

		if err := <-pubDone; err != nil {
			t.Fatalf("round %d: publish failed: %v", i, err)
		}

		select {
		case <-got:
		case <-time.After(300 * time.Millisecond):
			missed++
			t.Logf("round %d: publish during swap window was NOT observed within 300ms", i)
		}
	}

	if missed > 0 {
		t.Fatalf("REPRODUCED: %d/%d publishes racing a swapNC call were missed by the armed watch — "+
			"this is the mechanism, or at least a contributing one, behind mikle7/ppz#1's surfacing gap", missed, rounds)
	}
	t.Logf("all %d rounds observed — watch_registry's rearm survives a publish racing the swap window", rounds)
}
