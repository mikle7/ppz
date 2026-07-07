package daemon

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Wake watchdog — sleep/wake detection for the daemon.
//
// Go timers ride a monotonic clock that PAUSES while a macOS machine
// sleeps. After wake, every pending timer (the refresh loop's exp-30s
// fire, ticker-driven retries, …) resumes where it left off and fires
// LATE by up to its full remaining duration. In the 2026-06-11 incident
// (ppz-diag-20260611-073803.tgz) the JWT expired mid-sleep and the
// refresh timer fired ~2 minutes after wake — for that whole window the
// daemon had a dead NATS connection, an expired credential, and no
// recovery in flight.
//
// The watchdog detects wake by the one signal sleep can't hide: ticks
// are spaced by MONOTONIC time, timestamps by WALL time. In steady
// state consecutive ticks are ~interval of wall time apart; across a
// sleep, one tick arrives with a wall-clock gap of the whole sleep
// duration. When the gap exceeds interval+threshold, onWake fires with
// the observed gap.

// wakeTickInterval is the watchdog's tick cadence. Cheap (a clock read
// per tick), so frequent enough that wake detection adds at most one
// interval of latency on top of the wake itself.
const wakeTickInterval = 30 * time.Second

// wakeGapThreshold is the slack over the expected interval before a
// gap counts as a wake. Covers scheduler jitter and brief CPU
// starvation; a real sleep produces gaps of minutes-to-hours, so the
// classification margin is wide.
const wakeGapThreshold = time.Minute

// wakeRefreshRetryInterval is the default backoff between credential
// refresh attempts inside Daemon.onWake. Post-wake networks are flaky
// (DHCP/DNS/routes still settling — the incident's /auth/exchange
// failed for ~70s after wake while ICMP worked), so onWake retries
// until the exchange succeeds rather than giving up. Overridden in
// tests via Daemon.wakeRetryInterval.
const wakeRefreshRetryInterval = 5 * time.Second

type wakeWatchdog struct {
	interval  time.Duration
	threshold time.Duration
	now       func() time.Time
	onWake    func(gap time.Duration)
	last      time.Time
}

// newWakeWatchdog constructs a watchdog whose baseline is now().
// Production passes time.Now; tests inject a fake clock and drive
// tick() directly.
func newWakeWatchdog(interval, threshold time.Duration, now func() time.Time, onWake func(gap time.Duration)) *wakeWatchdog {
	return &wakeWatchdog{
		interval:  interval,
		threshold: threshold,
		now:       now,
		onWake:    onWake,
		last:      now(),
	}
}

// tick evaluates one tick: if more than interval+threshold of wall
// time passed since the previous tick, the machine slept (or was
// suspended) in between — fire onWake with the observed gap.
//
// Not safe for concurrent callers; run() is the only production
// driver. The baseline advances on EVERY tick, so one sleep fires
// onWake exactly once and the next tick is measured against the
// post-wake baseline.
func (w *wakeWatchdog) tick() {
	now := w.now()
	// Measure the gap on the WALL clock, NOT the monotonic clock. time.Now()
	// carries a monotonic reading, and time.Sub uses the monotonic delta
	// whenever both operands have one — but macOS suspends the monotonic clock
	// for the duration of a sleep. So a monotonic gap across a sleep reads as
	// only the awake time (~one tick interval) and the sleep is never seen.
	// Round(0) strips the monotonic reading from both operands, forcing the
	// subtraction onto the wall clock, which DOES advance across sleep. Without
	// this, the watchdog silently never fires on real hardware even though its
	// unit tests (which feed monotonic-free time.Date values) pass — the
	// 2026-06-30 incident logged a ~30s monotonic gap and zero wake events for
	// an 18-minute sleep.
	gap := now.Round(0).Sub(w.last.Round(0))
	w.last = now
	if gap > w.interval+w.threshold {
		w.onWake(gap)
	}
}

// run drives tick() from a real ticker until ctx is cancelled. The
// ticker itself pauses during system sleep — which is exactly the
// mechanism: the first post-wake tick carries the whole sleep as a
// wall-clock gap.
func (w *wakeWatchdog) run(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.tick()
		}
	}
}

// onWake is the daemon's wake handler: bring credentials and the NATS
// connection back NOW instead of waiting for paused timers to drain.
//
//  1. RefreshNowIfDue — after any sleep longer than the JWT's
//     remaining validity the credential is expired and NOTHING works
//     (NATS auth needs a fresh JWT, which needs /auth/exchange) until
//     a refresh succeeds. Transient failures are retried every
//     wakeRetryInterval — the post-wake network may take a while to
//     pass real traffic. ErrUnauthorized is terminal: the bearer was
//     revoked; retrying can't fix it (matches the refresh loop's own
//     semantics).
//  2. rebuildNC — swap in a fresh connection, attributed to
//     "wake-watchdog" in the event stream. No-ops via the generation
//     check if something else (OnRefreshed, ensureNATS) already
//     rebuilt it.
func (d *Daemon) onWake(ctx context.Context) error {
	if r := d.Refresh; r != nil {
		interval := d.wakeRetryInterval
		if interval <= 0 {
			interval = wakeRefreshRetryInterval
		}
		for {
			// Logged out (possibly mid-retry): nothing to refresh and
			// no credential to dial with — stop, matching
			// kickReconnect's per-iteration guard.
			if _, ok := d.State.Credentials(); !ok {
				return nil
			}
			_, err := r.RefreshNowIfDue(ctx, time.Now())
			if err == nil {
				break
			}
			if errors.Is(err, ErrUnauthorized) {
				return err
			}
			// Transient (the failure itself lands in the event stream
			// via the refresh loop's OnError hook) — back off and retry.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(interval):
			}
		}
	}
	return d.rebuildNC("wake-watchdog")
}

// startWakeWatchdog wires the watchdog into a running daemon: each
// detected wake is recorded as a "wake" event (so bundles from future
// incidents show exactly when the daemon noticed the sleep and how
// long it was) and recovery runs on its own goroutine — onWake retries
// can outlive a tick, and the watchdog must keep ticking meanwhile.
func (d *Daemon) startWakeWatchdog(ctx context.Context) {
	w := newWakeWatchdog(wakeTickInterval, wakeGapThreshold, time.Now, func(gap time.Duration) {
		d.recordNATSEvent(NATSEvent{
			Type:   "wake",
			At:     time.Now(),
			Caller: "wake-watchdog",
			JWTExp: d.Refresh.JWTExp(), // nil-safe; 0 pre-login
			Reason: fmt.Sprintf("wall-clock jump %s (system sleep)", gap.Round(time.Second)),
		})
		go func() { _ = d.onWake(ctx) }()
	})
	go w.run(ctx)
}
