package daemon

// Pattern detectors for the NATS connection-event stream.
//
// Detectors codify the "what does this trace mean?" knowledge from
// docs/diagnostics.md. Each is a pure function from a slice of events
// to a slice of PatternHit; the diagnostics CLI runs every detector
// against the visible window and prints each hit as a ⚠ line at the
// top of its default output.
//
// Adding a new pattern:
//  1. Write a detector func that returns []PatternHit.
//  2. Register it in detectors below.
//  3. Document the pattern in docs/diagnostics.md (name, what produces
//     it, what to do about it).
//
// Detectors must be deterministic and side-effect free — they run
// every time `ppz diagnostics` is invoked, and they're called from
// tests with synthetic event streams.

import (
	"fmt"
	"strings"
	"time"
)

// PatternHit is one match of one detector against the event window.
// Name is the stable detector identifier (kebab-case; doubles as the
// docs anchor). At is the timestamp of the first event in the match
// — used to sort hits chronologically. Detail is the human-readable
// one-liner shown in CLI output and the --json pattern array.
type PatternHit struct {
	Name   string    `json:"name"`
	At     time.Time `json:"at"`
	Detail string    `json:"detail"`
}

// detector is the signature every pattern function implements. Given a
// chronological slice of events, return zero or more hits. Detectors
// must tolerate empty input and events with zero-valued optional
// fields (caller, nc_id, jwt_exp) — the persisted jsonl from older
// schema versions can be missing those.
type detector func([]NATSEvent) []PatternHit

// detectors is the registered set, run in order. New patterns append
// here. Order doesn't matter for correctness; CLI output sorts hits
// by At before rendering.
var detectors = []detector{
	detectBurstSwapStorm,
	detectPostRotationAuthViolation,
}

// detectPatterns runs every registered detector and returns the
// combined hits in chronological order (At ascending). Empty result
// = clean trace; no warnings to surface.
func detectPatterns(events []NATSEvent) []PatternHit {
	var out []PatternHit
	for _, d := range detectors {
		out = append(out, d(events)...)
	}
	// Stable chronological sort (no time.Sort dependency to keep this
	// hot path trivially inline-able). Bubble is fine — pattern hits
	// per window are O(events / 100) in practice.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].At.Before(out[j-1].At); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// detectBurstSwapStorm fires when 3 or more "swap" events land within
// burstSwapWindow. That's the signature of concurrent ensureNATS
// callers racing with OnRefreshed (and each other) during a JWT
// rotation — Amplifier 1 + Amplifier 2 in the Phase 0 diagnosis.
// Each cluster of swaps becomes ONE hit (named for the first swap's
// time), so a trace with one storm at 00:33:30 doesn't render as 8
// separate warnings.
const (
	burstSwapWindow = 2 * time.Second
	burstSwapMin    = 3
)

func detectBurstSwapStorm(events []NATSEvent) []PatternHit {
	var hits []PatternHit
	i := 0
	for i < len(events) {
		if events[i].Type != "swap" {
			i++
			continue
		}
		// Scan forward through all events within burstSwapWindow,
		// counting only swaps. Non-swap events (e.g. disconnect/closed
		// emitted by nats.go during NC teardown) are skipped rather
		// than terminating the cluster scan — an interleaved disconnect
		// between two rapid swaps must not split the cluster below the
		// burstSwapMin threshold.
		start := i
		end := i
		count := 1
		for j := i + 1; j < len(events); j++ {
			if events[j].At.Sub(events[start].At) > burstSwapWindow {
				break
			}
			if events[j].Type == "swap" {
				count++
				end = j
			}
		}
		if count >= burstSwapMin {
			window := events[end].At.Sub(events[start].At)
			callers := uniqueCallers(events[start : end+1])
			hits = append(hits, PatternHit{
				Name:   "burst-swap-storm",
				At:     events[start].At,
				Detail: fmt.Sprintf("%d swaps in %s — concurrent NC mutation (callers: %s); see docs/diagnostics.md#burst-swap-storm", count, window.Round(100*time.Millisecond), strings.Join(callers, ", ")),
			})
			i = end + 1
			continue
		}
		i++
	}
	return hits
}

// uniqueCallers returns the distinct Caller values in events, in
// first-seen order. Used by burst-swap-storm to attribute the storm to
// the originating functions — typically OnRefreshed-callback +
// ensureNATS + ensureNATS-refresh-due when the diagnosis applies.
func uniqueCallers(events []NATSEvent) []string {
	seen := make(map[string]bool, len(events))
	var out []string
	for _, ev := range events {
		c := ev.Caller
		if c == "" {
			c = "?"
		}
		if seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}

// detectPostRotationAuthViolation fires on a "closed" event whose
// Reason names an Authorization Violation, when the preceding
// "disconnect" landed within authViolationProximity of the JWT's exp.
// That's the signature of nats.go's internal reconnect using a stale
// JWT because the daemon's proactive rotate didn't fire (or failed)
// before the server kicked at exp.
//
// Why not fire on every Authorization Violation? Some are unrelated
// to rotation (e.g. operator revoked the key mid-session). The
// near-exp constraint is what distinguishes "stale JWT after rotation
// failure" from "permission revoked".
const authViolationProximity = 60 * time.Second

func detectPostRotationAuthViolation(events []NATSEvent) []PatternHit {
	var hits []PatternHit
	for i, ev := range events {
		if ev.Type != "closed" {
			continue
		}
		if !strings.Contains(strings.ToLower(ev.Reason), "authorization violation") {
			continue
		}
		// Find the prior disconnect — it's where the server actually
		// kicked the connection; the closed event lands seconds later.
		var prior *NATSEvent
		for j := i - 1; j >= 0; j-- {
			if events[j].Type == "disconnect" {
				prior = &events[j]
				break
			}
		}
		if prior == nil || prior.JWTExp == 0 {
			continue
		}
		expTime := time.Unix(prior.JWTExp, 0)
		delta := prior.At.Sub(expTime)
		if delta < -authViolationProximity || delta > authViolationProximity {
			continue
		}
		hits = append(hits, PatternHit{
			Name: "post-rotation-auth-violation",
			At:   prior.At,
			Detail: fmt.Sprintf(
				"server-kick at jwt_exp ±%s → stale-jwt reconnect rejected; proactive rotate did not fire in time. See docs/diagnostics.md#post-rotation-auth-violation",
				delta.Round(time.Second),
			),
		})
	}
	return hits
}
