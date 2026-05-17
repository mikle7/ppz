package daemon

import (
	"testing"
	"time"
)

// classifyHeartbeatStatus is the pure tri-state rule used by `ppz who`:
//
//   < 1.5× interval   → online
//   < 3.0× interval   → stale (one beat missed; still likely alive)
//   >= 3.0× interval  → offline
//   never (zero time) → offline
//
// Boundaries are inclusive on the lower bound, exclusive on the upper —
// the test cases pin the contract.
func TestClassifyHeartbeatStatus(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		last     time.Time
		interval int
		want     string
	}{
		{"never-beat", time.Time{}, 60, "offline"},
		{"just-beat", now.Add(-1 * time.Second), 60, "online"},
		{"under-1.5x", now.Add(-89 * time.Second), 60, "online"},
		{"at-1.5x-boundary", now.Add(-90 * time.Second), 60, "stale"},
		{"under-3x", now.Add(-179 * time.Second), 60, "stale"},
		{"at-3x-boundary", now.Add(-180 * time.Second), 60, "offline"},
		{"way-past-3x", now.Add(-1 * time.Hour), 60, "offline"},
		// Future-dated beat (skewed clock) is treated as online — defensive
		// against minor clock drift between agent and daemon.
		{"future-beat", now.Add(5 * time.Second), 60, "online"},
		// Different intervals scale the thresholds proportionally.
		// 30s interval: 1.5× = 45s, 3× = 90s.
		{"30s-interval-online", now.Add(-30 * time.Second), 30, "online"},
		{"30s-interval-stale", now.Add(-50 * time.Second), 30, "stale"},
		{"30s-interval-offline", now.Add(-91 * time.Second), 30, "offline"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifyHeartbeatStatus(c.last, now, c.interval)
			if got != c.want {
				t.Errorf("classifyHeartbeatStatus(last=%v, now=%v, interval=%d) = %q, want %q",
					c.last, now, c.interval, got, c.want)
			}
		})
	}
}

// Zero/negative interval is treated as 60s (the default) so a malformed
// payload doesn't crash the classifier.
func TestClassifyHeartbeatStatus_ZeroIntervalFallsBackTo60s(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	if got := classifyHeartbeatStatus(now.Add(-1*time.Second), now, 0); got != "online" {
		t.Errorf("zero interval → online for fresh beat, got %q", got)
	}
	if got := classifyHeartbeatStatus(now.Add(-200*time.Second), now, -1); got != "offline" {
		t.Errorf("negative interval → offline for stale beat, got %q", got)
	}
}
