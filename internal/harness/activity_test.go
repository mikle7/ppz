package harness

import (
	"testing"
	"time"
)

// All activity tests share a fixed identification time; `base` is
// comfortably past the 3s startup grace so observations there are
// judged purely on causality.
var (
	trackerStart = time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	base         = trackerStart.Add(4 * time.Second)
)

// A freshly identified harness with no output yet is idle, not unknown
// — identification already happened; this tracker only answers
// working-vs-idle.
func TestActivity_FreshTrackerIsIdle(t *testing.T) {
	tr := NewActivityTracker(trackerStart)
	if got := tr.State(base); got != StateIdle {
		t.Errorf("State = %q, want %q", got, StateIdle)
	}
}

// Output with no recent user input is agent work: working while inside
// the 1800ms activity window, idle once the window expires. The window
// IS the working→idle hysteresis — no separate confirmation machinery.
func TestActivity_UntaintedOutputMarksWorkingUntilWindowExpires(t *testing.T) {
	tr := NewActivityTracker(trackerStart)
	tr.ObserveOutput(base)

	if got := tr.State(base.Add(500 * time.Millisecond)); got != StateWorking {
		t.Errorf("500ms after output: State = %q, want %q", got, StateWorking)
	}
	if got := tr.State(base.Add(2 * time.Second)); got != StateIdle {
		t.Errorf("2s after output (window expired): State = %q, want %q", got, StateIdle)
	}
}

// Output arriving within the 1200ms input-taint window is presumed to
// be local echo / prompt redraw caused by the user's keystrokes, not
// agent work. Without this, every character typed would flash the
// session to working.
func TestActivity_OutputDuringInputTaintStaysIdle(t *testing.T) {
	tr := NewActivityTracker(trackerStart)
	tr.ObserveInput(base)
	tr.ObserveOutput(base.Add(300 * time.Millisecond))

	if got := tr.State(base.Add(400 * time.Millisecond)); got != StateIdle {
		t.Errorf("State = %q, want %q (echo must not read as work)", got, StateIdle)
	}
}

// Once the taint window expires, output is the agent responding to the
// submitted input — the user pressed enter, the harness started
// streaming. That must read as working.
func TestActivity_OutputAfterTaintExpiryMarksWorking(t *testing.T) {
	tr := NewActivityTracker(trackerStart)
	tr.ObserveInput(base)
	tr.ObserveOutput(base.Add(1500 * time.Millisecond)) // past 1200ms taint

	if got := tr.State(base.Add(1600 * time.Millisecond)); got != StateWorking {
		t.Errorf("State = %q, want %q (post-taint output is agent work)", got, StateWorking)
	}
}

// Output during the 3s startup grace is boot noise (banner, prompt
// paint) and must not create a working state — even when checked right
// after the grace expires, while that output is still inside what would
// otherwise be its activity window.
func TestActivity_StartupGraceSuppressesBootNoise(t *testing.T) {
	tr := NewActivityTracker(trackerStart)
	tr.ObserveOutput(trackerStart.Add(2500 * time.Millisecond)) // inside grace

	if got := tr.State(trackerStart.Add(2900 * time.Millisecond)); got != StateIdle {
		t.Errorf("inside grace: State = %q, want %q", got, StateIdle)
	}
	if got := tr.State(trackerStart.Add(3200 * time.Millisecond)); got != StateIdle {
		t.Errorf("just past grace: State = %q, want %q (grace output must not count)", got, StateIdle)
	}
}

// Sustained typing — user composing a prompt, each keystroke echoed —
// keeps refreshing the taint window, so the session never reads as
// working no matter how long it goes on.
func TestActivity_SustainedTypingNeverWorking(t *testing.T) {
	tr := NewActivityTracker(trackerStart)
	for i := 0; i < 6; i++ {
		at := base.Add(time.Duration(i) * 500 * time.Millisecond)
		tr.ObserveInput(at)
		tr.ObserveOutput(at.Add(100 * time.Millisecond)) // echo
	}

	if got := tr.State(base.Add(3100 * time.Millisecond)); got != StateIdle {
		t.Errorf("State = %q, want %q (typing+echo is not agent work)", got, StateIdle)
	}
}
