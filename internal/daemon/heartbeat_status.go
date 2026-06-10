package daemon

import "time"

// ClassifyHeartbeatStatus is the exported alias of classifyHeartbeatStatus
// for callers outside the daemon package (currently `ppz who`'s renderer
// in internal/cli). Keeping the rule in one place ensures the colour
// boundaries and the filter boundaries can never drift apart.
func ClassifyHeartbeatStatus(last time.Time, now time.Time, intervalSec int) string {
	return classifyHeartbeatStatus(last, now, intervalSec)
}

// CombineHeartbeatStatus merges liveness with the beat's agent state
// into the single STATUS string `ppz who`'s table shows:
//
//	offline, *        → "offline"           (state too old to be meaningful)
//	online,  ""       → "online"            (plain shell / no harness)
//	online,  working  → "online|working"
//	stale,   working  → "stale|working"     (amber colour conveys the doubt)
//
// Kept next to the liveness classifier for the same reason that rule is
// here: one source of truth, so the renderer and any future filters
// can't drift apart.
func CombineHeartbeatStatus(liveness string, agentState string) string {
	if agentState == "" || liveness == "offline" {
		return liveness
	}
	return liveness + "|" + agentState
}

// classifyHeartbeatStatus is the tri-state rule that drives `ppz who`'s
// online / stale / offline column. Pure function so the colour and
// filter logic upstream can rely on a single source of truth.
//
//	< 1.5× interval   → "online"   (green)
//	< 3.0× interval   → "stale"    (amber) — at least one missed beat
//	>= 3.0× interval  → "offline"  (red)   — confidently dead
//	zero last         → "offline"
//
// Future-dated beats (clock skew between agent and daemon) are treated
// as online to avoid surprise reds during boot-time drift.
func classifyHeartbeatStatus(last time.Time, now time.Time, intervalSec int) string {
	if last.IsZero() {
		return "offline"
	}
	if intervalSec <= 0 {
		intervalSec = 60
	}
	interval := time.Duration(intervalSec) * time.Second
	age := now.Sub(last)
	if age < 0 {
		return "online"
	}
	if 2*age < 3*interval {
		return "online"
	}
	if age < 3*interval {
		return "stale"
	}
	return "offline"
}
