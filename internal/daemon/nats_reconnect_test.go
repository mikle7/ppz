package daemon

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// startEmbeddedNATSURL spins up an in-process, no-auth NATS server and
// returns its client URL. Test-only; shut down via t.Cleanup.
func startEmbeddedNATSURL(t *testing.T) string {
	t.Helper()
	s, err := natsserver.NewServer(&natsserver.Options{
		Host: "127.0.0.1",
		Port: -1, // ephemeral
	})
	if err != nil {
		t.Fatalf("new embedded nats: %v", err)
	}
	go s.Start()
	if !s.ReadyForConnections(5 * time.Second) {
		s.Shutdown()
		t.Fatalf("embedded nats not ready")
	}
	t.Cleanup(s.Shutdown)
	return s.ClientURL()
}

// TestRebuildNC_ConcurrentCallersCoalesce pins the burst-swap-storm fix:
// when many goroutines (the refresh-loop OnRefreshed callback + every
// in-flight ensureNATS caller) rebuild the NATS connection at once — as
// they do around a JWT rotation — they must coalesce into a SINGLE
// reconnect+swap, not N racing swaps that close each other's connections.
//
// RED until rebuildNC is serialized: today each caller observes the stale
// NC and dials+swaps independently, producing a swap per goroutine and
// tripping the burst-swap-storm detector.
func TestRebuildNC_ConcurrentCallersCoalesce(t *testing.T) {
	url := startEmbeddedNATSURL(t)

	var dials int64
	d := &Daemon{
		State:      NewState(t.TempDir()),
		NATSEvents: newNATSEventRing(natsEventRingCap),
		Follows:    newFollowRegistry(),
		NATSURL:    url,
		// Stub dial: real connection to the embedded server (so
		// IsConnected() is genuinely true) while counting calls. Refresh
		// is nil → JWTExp() == 0, ncExp starts 0, so once one caller
		// connects the generation matches and the rest must no-op.
		dial: func(u string, _ *RefreshLoop, _ func(NATSEvent)) (*nats.Conn, error) {
			atomic.AddInt64(&dials, 1)
			return nats.Connect(u)
		},
	}

	const goroutines = 12
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = d.rebuildNC("ensureNATS")
		}()
	}
	wg.Wait()
	if d.NC != nil {
		t.Cleanup(func() { d.NC.Close() })
	}

	events := d.NATSEvents.Snapshot()
	swaps := 0
	for _, e := range events {
		if e.Type == "swap" {
			swaps++
		}
	}
	if hits := detectBurstSwapStorm(events); len(hits) > 0 {
		t.Errorf("burst-swap-storm detected (%d hits) — concurrent rebuildNC did not coalesce: %+v", len(hits), hits)
	}
	if swaps != 1 {
		t.Errorf("got %d swap events from %d concurrent rebuildNC calls, want 1 (must coalesce)", swaps, goroutines)
	}
	if n := atomic.LoadInt64(&dials); n != 1 {
		t.Errorf("got %d dials, want 1 (a single reconnect per rotation)", n)
	}
}
