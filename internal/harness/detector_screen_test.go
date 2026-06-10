package harness

import (
	"testing"
	"time"
)

// claudeDetectorWithScreen returns a Detector that identified claude at
// detStart+1s, with a settable screen source.
func claudeDetectorWithScreen(content *string) *Detector {
	proc := ForegroundProc{PID: 4242, Comm: "claude", Argv: []string{"claude"}}
	d := NewDetector(func() (ForegroundProc, error) { return proc, nil })
	d.SetScreen(func() string { return *content })
	d.Poll(detStart.Add(1 * time.Second))
	return d
}

// An identified harness that is byte-idle but shows blocker chrome on
// screen is blocked — the state `ppz who` exists to surface, since it
// means a human needs to go answer something.
func TestDetector_IdleWithBlockerScreenIsBlocked(t *testing.T) {
	content := claudeBashPermission
	d := claudeDetectorWithScreen(&content)

	// Well past startup grace, no output observed: byte-idle.
	if got := d.Snapshot(detStart.Add(10 * time.Second)).State; got != StateBlocked {
		t.Errorf("State = %q, want %q", got, StateBlocked)
	}
}

// PTY activity is the authority for working (same arbitration as
// herdr): fresh untainted output wins over blocker chrome — stale
// dialog text must not mask an actively-streaming agent.
func TestDetector_WorkingOverridesBlockerScreen(t *testing.T) {
	content := claudeBashPermission
	d := claudeDetectorWithScreen(&content)

	at := detStart.Add(10 * time.Second)
	d.ObserveOutput(at)
	if got := d.Snapshot(at.Add(500 * time.Millisecond)).State; got != StateWorking {
		t.Errorf("State = %q, want %q (activity beats screen)", got, StateWorking)
	}
}

// When the dialog is answered and the screen returns to the idle
// prompt, blocked clears on the next snapshot.
func TestDetector_BlockedClearsWhenScreenClears(t *testing.T) {
	content := claudeBashPermission
	d := claudeDetectorWithScreen(&content)

	at := detStart.Add(10 * time.Second)
	if got := d.Snapshot(at).State; got != StateBlocked {
		t.Fatalf("setup: State = %q, want %q", got, StateBlocked)
	}

	content = claudeIdlePrompt
	if got := d.Snapshot(at.Add(time.Second)).State; got != StateIdle {
		t.Errorf("State after dialog answered = %q, want %q", got, StateIdle)
	}
}

// No screen source wired (ppz built without the live screen model, or
// it failed to start): blocked is simply never reported. Phase-2
// behavior is the fallback, not an error.
func TestDetector_NoScreenSourceStaysIdle(t *testing.T) {
	proc := ForegroundProc{PID: 4242, Comm: "claude", Argv: []string{"claude"}}
	d := NewDetector(func() (ForegroundProc, error) { return proc, nil })
	d.Poll(detStart.Add(1 * time.Second))

	if got := d.Snapshot(detStart.Add(10 * time.Second)).State; got != StateIdle {
		t.Errorf("State = %q, want %q", got, StateIdle)
	}
}

// A harness without a ScreenDetector (everything but claude in phase
// 3) never reports blocked, even with blocker-looking text on screen.
func TestDetector_NonClaudeBlockerTextStaysIdle(t *testing.T) {
	proc := ForegroundProc{PID: 4242, Comm: "pi", Argv: []string{"pi"}}
	d := NewDetector(func() (ForegroundProc, error) { return proc, nil })
	content := claudeBashPermission
	d.SetScreen(func() string { return content })
	d.Poll(detStart.Add(1 * time.Second))

	if got := d.Snapshot(detStart.Add(10 * time.Second)).State; got != StateIdle {
		t.Errorf("State = %q, want %q (pi has no screen detector)", got, StateIdle)
	}
}

// Blocked is allowed during the startup grace: the grace exists to
// stop boot noise reading as *working*, but a permission prompt right
// at boot (claude --resume straight onto a dialog) is a real, strong
// signal — suppressing it would hide exactly the state the user most
// needs to see.
func TestDetector_BlockedDuringStartupGrace(t *testing.T) {
	content := claudeBashPermission
	d := claudeDetectorWithScreen(&content)

	// 1s after identification: inside the 3s grace.
	if got := d.Snapshot(detStart.Add(2 * time.Second)).State; got != StateBlocked {
		t.Errorf("State = %q, want %q (blocked is grace-exempt)", got, StateBlocked)
	}
}
