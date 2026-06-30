package daemon

import (
	"reflect"
	"testing"
	"time"
	"unsafe"
)

// forgeSleptTime returns base with its WALL clock advanced by wallJump but
// its MONOTONIC reading advanced by only monoJump — the exact signature of
// a macOS sleep, where the wall clock tracks real time but Go's monotonic
// clock (mach_absolute_time) is suspended for the duration of the sleep.
//
// Why this is needed: time.Now() carries a monotonic reading; time.Date()
// and time.Unix() do not. time.Time.Sub uses the monotonic delta whenever
// BOTH operands carry one, so production code that does now.Sub(last) on two
// time.Now() values measures only the AWAKE-time delta across a sleep, never
// the wall-clock gap. The existing watchdog tests feed monotonic-free
// time.Date values, so they exercise the wall-clock fallback and never see
// this — which is exactly why the bug shipped green.
//
// We advance the wall clock through the public Add (so its packed
// representation stays valid) and then roll the monotonic field (ext) back
// via reflection. ext is the only field touched; the wall clock is left as
// Add produced it.
func forgeSleptTime(base time.Time, wallJump, monoJump time.Duration) time.Time {
	t := base.Add(wallJump) // wall += wallJump, monotonic += wallJump
	ext := reflect.ValueOf(&t).Elem().FieldByName("ext")
	extPtr := (*int64)(unsafe.Pointer(ext.UnsafeAddr()))
	*extPtr -= int64(wallJump - monoJump) // leave monotonic only monoJump ahead
	return t
}

// TestForgeSleptTime_HasDivergentWallAndMonotonic is a self-check on the
// fixture: it must produce a time whose WALL clock moved by wallJump while
// its MONOTONIC reading moved by only monoJump. If a future Go release
// changes the time.Time internals this guard fails loudly here instead of
// silently neutering the sleep test below.
func TestForgeSleptTime_HasDivergentWallAndMonotonic(t *testing.T) {
	base := time.Now()
	slept := forgeSleptTime(base, 18*time.Minute, 30*time.Second)

	if wall := time.Duration(slept.UnixNano() - base.UnixNano()); wall != 18*time.Minute {
		t.Fatalf("forged wall jump = %v, want 18m (UnixNano is wall-clock)", wall)
	}
	if mono := slept.Sub(base); mono != 30*time.Second {
		t.Fatalf("forged monotonic jump = %v, want 30s (Sub is monotonic when both carry it)", mono)
	}
}

// TestWakeWatchdog_FiresWhenMonotonicClockFrozenDuringSleep reproduces the
// 2026-06-30 incident (ppz-diag-20260630-184330.tgz): the Mac slept ~18 min
// (NATS event log shows the last pre-sleep event at 18:15:18 and the first
// post-wake disconnect at 18:33:19), the JWT expired mid-sleep (exp 18:20:16),
// and the post-wake NATS connection limped on the expired credential through
// repeated "Authorization Violation" for ~9 minutes until nats.go finally
// gave up and the background reconnect recovered it at 18:42:36.
//
// The wake watchdog — added in v0.48.1 specifically to make wake recovery
// near-instant, and present in the user's running v0.50.3 — recorded ZERO
// "wake" events across that entire sleep. It never fired.
//
// Root cause: wakeWatchdog.tick() computes the inter-tick gap with
// now.Sub(w.last). In production both values come from time.Now() and so
// carry monotonic readings, making Sub return the MONOTONIC delta. Go's
// monotonic clock is suspended while macOS sleeps, so across an 18-minute
// sleep the gap reads as roughly one tick interval — never exceeding
// interval+threshold. onWake is never called, and the daemon falls back to
// waiting out nats.go's own reconnect budget (the ~9-minute outage) instead
// of refreshing the JWT and rebuilding immediately.
//
// RED: tick() uses monotonic Sub, so gap ≈ 30s < 90s and onWake never fires.
// GREEN: tick() measures the wall-clock gap (strip monotonic, e.g. via
// now.Round(0)) so the 18-minute wall jump is seen and onWake fires.
func TestWakeWatchdog_FiresWhenMonotonicClockFrozenDuringSleep(t *testing.T) {
	const interval = 30 * time.Second
	const threshold = time.Minute

	base := time.Now() // a real monotonic-bearing reading, exactly like production.

	// First read = the watchdog baseline (consumed by newWakeWatchdog).
	// Second read = the first post-wake tick: 18 minutes of WALL time have
	// elapsed, but the monotonic clock advanced only one tick interval
	// because it was suspended for the sleep.
	slept := forgeSleptTime(base, 18*time.Minute, interval)
	reads := []time.Time{base, slept}
	i := 0
	now := func() time.Time {
		r := reads[i]
		if i < len(reads)-1 {
			i++
		}
		return r
	}

	var fires int
	var gotGap time.Duration
	w := newWakeWatchdog(interval, threshold, now, func(gap time.Duration) {
		fires++
		gotGap = gap
	})

	w.tick() // the first post-wake tick

	if fires != 1 {
		t.Fatalf("onWake fired %d times after an 18-minute wall-clock sleep, want 1; "+
			"tick() is measuring the inter-tick gap with the monotonic clock, which is "+
			"suspended during macOS sleep, so real sleeps go undetected — exactly why "+
			"ppz-diag-20260630 logged zero 'wake' events across an 18-minute sleep", fires)
	}
	if gotGap < 9*time.Minute {
		t.Fatalf("onWake gap = %v, want the wall-clock gap (~18m); a small gap means "+
			"monotonic time leaked into the measurement", gotGap)
	}
}

// TestWakeWatchdog_SteadyCadenceWithMonotonicTimesNeverFires guards the fix
// against over-firing: with monotonic-bearing times (as in production) and a
// normal awake cadence — each tick one interval of BOTH wall and monotonic
// time after the last — the watchdog must stay silent. A naive "always use
// wall clock but mis-handle the baseline" fix could regress this.
func TestWakeWatchdog_SteadyCadenceWithMonotonicTimesNeverFires(t *testing.T) {
	const interval = 30 * time.Second
	const threshold = time.Minute

	cur := time.Now()
	now := func() time.Time { return cur }

	var fires int
	w := newWakeWatchdog(interval, threshold, now, func(time.Duration) { fires++ })

	// 20 ordinary ticks: advance BOTH wall and monotonic by one interval
	// (plus mild jitter) each time — no sleep, so nothing may fire.
	for n := 0; n < 20; n++ {
		cur = cur.Add(interval + 500*time.Millisecond)
		w.tick()
	}
	if fires != 0 {
		t.Fatalf("onWake fired %d times under steady awake cadence, want 0", fires)
	}
}
