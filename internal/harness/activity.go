package harness

import "time"

// Timing constants ported verbatim from herdr
// (src/pane/agent_detection.rs). The output window doubles as the
// working→idle hysteresis; the taint window keeps local echo of user
// keystrokes from reading as agent work; the grace window suppresses
// harness boot noise right after identification.
const (
	outputActivityWindow = 1800 * time.Millisecond
	inputTaintWindow     = 1200 * time.Millisecond
	startupGraceWindow   = 3 * time.Second
)

// ActivityTracker classifies a wrapped harness as working or idle from
// PTY byte causality alone — no screen parsing. Output observed while
// no recent user input is in flight marks the harness working for
// outputActivityWindow; quiescence past the window reads as idle.
// Callers pass `now` explicitly so tests never sleep.
//
// Not safe for concurrent use; the wrapper serializes observations onto
// its detection goroutine.
type ActivityTracker struct {
	start        time.Time
	lastInput    time.Time
	lastAgentOut time.Time // most recent output attributed to the agent
}

// NewActivityTracker starts tracking for a harness identified at
// `start`. The first startupGraceWindow after start reads as idle
// regardless of output.
func NewActivityTracker(start time.Time) *ActivityTracker {
	return &ActivityTracker{start: start}
}

// ObserveOutput records PTY output bytes from the child at `now`.
// Output inside the startup grace is boot noise, and output inside the
// input-taint window is presumed local echo / prompt redraw; neither
// counts as agent work.
func (t *ActivityTracker) ObserveOutput(now time.Time) {
	if now.Sub(t.start) < startupGraceWindow {
		return
	}
	if !t.lastInput.IsZero() && now.Sub(t.lastInput) < inputTaintWindow {
		return
	}
	if now.After(t.lastAgentOut) {
		t.lastAgentOut = now
	}
}

// ObserveInput records user-originated input at `now` (local stdin,
// remote .stdin messages, alert auto-submission).
func (t *ActivityTracker) ObserveInput(now time.Time) {
	if now.After(t.lastInput) {
		t.lastInput = now
	}
}

// State returns StateWorking or StateIdle as of `now`: working while
// agent-attributed output is fresher than outputActivityWindow, idle
// otherwise. The window doubles as the working→idle hysteresis.
func (t *ActivityTracker) State(now time.Time) State {
	if !t.lastAgentOut.IsZero() && now.Sub(t.lastAgentOut) < outputActivityWindow {
		return StateWorking
	}
	return StateIdle
}
