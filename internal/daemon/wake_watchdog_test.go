package daemon

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

// RED for the 2026-06-11 wake-from-sleep incident
// (ppz-diag-20260611-073803.tgz).
//
// Go timers on macOS are driven by a monotonic clock that PAUSES while
// the machine sleeps. In the incident, the refresh loop's timer was set
// at 07:26:40 to fire 4m30s later; the Mac slept at ~07:29 and woke at
// ~07:36:50, but the timer — having "experienced" only ~2½ minutes —
// did not fire until 07:38:48. The JWT had expired mid-sleep (07:31:40),
// so for the entire post-wake window the daemon had a dead NATS
// connection, an expired credential, and no background recovery in
// flight. Total user-visible outage after wake: ~2 minutes.
//
// These tests pin the fix's contract — a wake watchdog that detects
// sleep by observing wall-clock jumps between ticks, plus a Daemon
// hook that immediately refreshes credentials (retrying transient
// failures) and rebuilds the NATS connection:
//
//   - newWakeWatchdog(interval, threshold, now, onWake) *wakeWatchdog
//     interval:  the expected tick cadence (driven by run's ticker).
//     threshold: slack added to interval before a gap counts as a wake.
//     now:       clock source (injectable; production passes time.Now).
//     onWake:    fired with the observed gap when a tick arrives more
//                than interval+threshold of WALL time after the
//                previous tick. The macOS sleep signature is exactly
//                this: ticks are spaced by monotonic time, so a sleep
//                shows up as one tick with a huge wall-clock gap.
//   - (*wakeWatchdog).tick() — evaluate one tick (run() drives this
//     from a real time.Ticker; tests drive it directly).
//   - (*Daemon).onWake(ctx) — what the daemon wires into onWake:
//     RefreshNowIfDue with retries on transient errors (the incident's
//     post-wake /auth/exchange failures), then rebuildNC, attributed
//     in the event stream as caller "wake-watchdog".

func TestWakeWatchdog_DetectsWallClockJump(t *testing.T) {
	t0 := time.Date(2026, 6, 11, 7, 26, 40, 0, time.UTC)
	clock := t0
	now := func() time.Time { return clock }

	var gotGap time.Duration
	var fires int
	w := newWakeWatchdog(30*time.Second, time.Minute, now, func(gap time.Duration) {
		fires++
		gotGap = gap
	})

	// One normal tick: 30s of wall time elapsed, as the ticker expects.
	clock = clock.Add(30 * time.Second)
	w.tick()
	if fires != 0 {
		t.Fatalf("onWake fired on a normal 30s tick")
	}

	// Sleep: the next tick arrives 10 minutes of wall time later (the
	// machine was asleep; the ticker only experienced its 30s of
	// monotonic time).
	clock = clock.Add(10 * time.Minute)
	w.tick()
	if fires != 1 {
		t.Fatalf("onWake fired %d times after a 10m wall-clock jump, want 1", fires)
	}
	if gotGap < 9*time.Minute {
		t.Fatalf("onWake gap = %v, want the observed wall-clock gap (~10m)", gotGap)
	}

	// Cadence resumes: the tick after the wake is normal again.
	clock = clock.Add(30 * time.Second)
	w.tick()
	if fires != 1 {
		t.Fatalf("onWake re-fired on a normal post-wake tick")
	}
}

func TestWakeWatchdog_SteadyCadenceNeverFires(t *testing.T) {
	clock := time.Date(2026, 6, 11, 7, 0, 0, 0, time.UTC)
	now := func() time.Time { return clock }

	var fires int
	w := newWakeWatchdog(30*time.Second, time.Minute, now, func(time.Duration) { fires++ })

	// An hour of ordinary ticks, including mild scheduler jitter — none
	// may be misread as a wake.
	for i := 0; i < 120; i++ {
		clock = clock.Add(30*time.Second + 500*time.Millisecond)
		w.tick()
	}
	if fires != 0 {
		t.Fatalf("onWake fired %d times under steady cadence, want 0", fires)
	}
}

// TestDaemonOnWake_RefreshesAndRebuilds pins the daemon-side wiring: at
// wake with an expired JWT (the incident state), onWake must refresh
// credentials immediately and swap in a fresh NATS connection,
// attributed to caller "wake-watchdog" in the event stream.
func TestDaemonOnWake_RefreshesAndRebuilds(t *testing.T) {
	url := startEmbeddedNATSURL(t)

	var refreshes int64
	d := &Daemon{
		State:      NewState(t.TempDir()),
		NATSEvents: newNATSEventRing(natsEventRingCap),
		Follows:    newFollowRegistry(),
		NATSURL:    url,
		dial: func(u string, _ *RefreshLoop, _ func(NATSEvent)) (*nats.Conn, error) {
			return nats.Connect(u)
		},
	}
	d.Refresh = &RefreshLoop{
		AccountID: "00000000-0000-0000-0000-000000000001",
		Refresh: func(context.Context, string) (string, string, int64, error) {
			atomic.AddInt64(&refreshes, 1)
			return "fresh-jwt", "fresh-seed", time.Now().Add(5 * time.Minute).Unix(), nil
		},
	}
	// Expired credential, no live NC — the post-wake incident state.
	if err := d.Refresh.Start(context.Background(), "stale-jwt", "stale-seed",
		time.Now().Add(-time.Minute).Unix()); err != nil {
		t.Fatalf("RefreshLoop.Start: %v", err)
	}
	t.Cleanup(d.Refresh.Stop)

	if err := d.onWake(context.Background()); err != nil {
		t.Fatalf("onWake: %v", err)
	}
	t.Cleanup(func() {
		if d.NC != nil {
			d.NC.Close()
		}
	})

	if atomic.LoadInt64(&refreshes) == 0 {
		t.Fatalf("onWake did not refresh the expired credential")
	}
	if d.NC == nil || !d.NC.IsConnected() {
		t.Fatalf("onWake did not rebuild the NATS connection")
	}
	var attributed bool
	for _, ev := range d.NATSEvents.Snapshot() {
		if ev.Type == "swap" && ev.Caller == "wake-watchdog" {
			attributed = true
		}
	}
	if !attributed {
		t.Fatalf("no swap event attributed to wake-watchdog; events: %+v", d.NATSEvents.Snapshot())
	}
}

// TestDaemonOnWake_RetriesTransientRefreshFailures pins the retry
// contract: in the incident, /auth/exchange kept failing for ~70s after
// wake (the network wasn't ready), and recovery had to wait for an
// unrelated timer. onWake must keep retrying transient refresh failures
// itself — backing off via d.wakeRetryInterval (injectable; production
// default a few seconds) — and connect as soon as an attempt succeeds.
func TestDaemonOnWake_RetriesTransientRefreshFailures(t *testing.T) {
	url := startEmbeddedNATSURL(t)

	var attempts int64
	d := &Daemon{
		State:             NewState(t.TempDir()),
		NATSEvents:        newNATSEventRing(natsEventRingCap),
		Follows:           newFollowRegistry(),
		NATSURL:           url,
		wakeRetryInterval: 5 * time.Millisecond,
		dial: func(u string, _ *RefreshLoop, _ func(NATSEvent)) (*nats.Conn, error) {
			return nats.Connect(u)
		},
	}
	d.Refresh = &RefreshLoop{
		AccountID: "00000000-0000-0000-0000-000000000001",
		Refresh: func(context.Context, string) (string, string, int64, error) {
			if atomic.AddInt64(&attempts, 1) < 3 {
				return "", "", 0, errors.New("dial tcp: connect: network is unreachable")
			}
			return "fresh-jwt", "fresh-seed", time.Now().Add(5 * time.Minute).Unix(), nil
		},
	}
	if err := d.Refresh.Start(context.Background(), "stale-jwt", "stale-seed",
		time.Now().Add(-time.Minute).Unix()); err != nil {
		t.Fatalf("RefreshLoop.Start: %v", err)
	}
	t.Cleanup(d.Refresh.Stop)

	if err := d.onWake(context.Background()); err != nil {
		t.Fatalf("onWake after transient failures: %v", err)
	}
	t.Cleanup(func() {
		if d.NC != nil {
			d.NC.Close()
		}
	})

	if got := atomic.LoadInt64(&attempts); got < 3 {
		t.Fatalf("onWake gave up after %d refresh attempts, want retries until success", got)
	}
	if d.NC == nil || !d.NC.IsConnected() {
		t.Fatalf("onWake did not rebuild the NATS connection once refresh succeeded")
	}
}

// TestDaemonOnWake_StopsOnUnauthorized — a revoked bearer must NOT be
// retried forever: ErrUnauthorized is terminal (matching the refresh
// loop's own semantics), so a wake on a logged-out machine fails fast.
func TestDaemonOnWake_StopsOnUnauthorized(t *testing.T) {
	var attempts int64
	d := &Daemon{
		State:             NewState(t.TempDir()),
		NATSEvents:        newNATSEventRing(natsEventRingCap),
		Follows:           newFollowRegistry(),
		NATSURL:           "nats://127.0.0.1:1",
		wakeRetryInterval: time.Millisecond,
	}
	d.Refresh = &RefreshLoop{
		AccountID: "00000000-0000-0000-0000-000000000001",
		Refresh: func(context.Context, string) (string, string, int64, error) {
			atomic.AddInt64(&attempts, 1)
			return "", "", 0, ErrUnauthorized
		},
	}
	if err := d.Refresh.Start(context.Background(), "stale-jwt", "stale-seed",
		time.Now().Add(-time.Minute).Unix()); err != nil {
		t.Fatalf("RefreshLoop.Start: %v", err)
	}
	t.Cleanup(d.Refresh.Stop)

	err := d.onWake(context.Background())
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("onWake = %v, want ErrUnauthorized", err)
	}
	if got := atomic.LoadInt64(&attempts); got > 2 {
		t.Fatalf("onWake retried an unauthorized refresh %d times — must stop immediately", got)
	}
}
