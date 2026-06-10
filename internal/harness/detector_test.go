package harness

import (
	"errors"
	"testing"
	"time"
)

var detStart = time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

// fakeInspector is a settable foreground-process source standing in for
// the TIOCGPGRP + process-lookup seam.
type fakeInspector struct {
	proc ForegroundProc
	err  error
}

func (f *fakeInspector) inspect() (ForegroundProc, error) {
	return f.proc, f.err
}

// Polling a PTY whose foreground group is a known harness identifies
// it: harness name, the foreground pid, and an initial idle state.
func TestDetector_IdentifiesForegroundHarness(t *testing.T) {
	ins := &fakeInspector{proc: ForegroundProc{PID: 4242, Comm: "claude", Argv: []string{"claude"}}}
	d := NewDetector(ins.inspect, detStart)

	d.Poll(detStart.Add(1 * time.Second))
	snap := d.Snapshot(detStart.Add(1 * time.Second))

	if snap.Harness != "claude" {
		t.Errorf("Harness = %q, want claude", snap.Harness)
	}
	if snap.ChildPID != 4242 {
		t.Errorf("ChildPID = %d, want 4242", snap.ChildPID)
	}
	if snap.State != StateIdle {
		t.Errorf("State = %q, want %q", snap.State, StateIdle)
	}
}

// When the harness exits back to the shell, the next successful poll
// clears the detection entirely — `ppz who` must stop showing a harness
// the moment the foreground is a plain shell again.
func TestDetector_ClearsWhenHarnessExitsToShell(t *testing.T) {
	ins := &fakeInspector{proc: ForegroundProc{PID: 4242, Comm: "claude", Argv: []string{"claude"}}}
	d := NewDetector(ins.inspect, detStart)
	d.Poll(detStart.Add(1 * time.Second))

	ins.proc = ForegroundProc{PID: 4000, Comm: "zsh", Argv: []string{"zsh"}}
	d.Poll(detStart.Add(2 * time.Second))
	snap := d.Snapshot(detStart.Add(2 * time.Second))

	if snap.Harness != "" || snap.ChildPID != 0 || snap.State != StateUnknown {
		t.Errorf("Snapshot = %+v, want zero Detection after harness exit", snap)
	}
}

// A transient inspect failure (ps hiccup, racing process exit) must not
// flap the detection — keep the last successful identification until a
// successful poll says otherwise.
func TestDetector_RetainsIdentificationOnInspectError(t *testing.T) {
	ins := &fakeInspector{proc: ForegroundProc{PID: 4242, Comm: "claude", Argv: []string{"claude"}}}
	d := NewDetector(ins.inspect, detStart)
	d.Poll(detStart.Add(1 * time.Second))

	ins.err = errors.New("ps: exit 1")
	d.Poll(detStart.Add(2 * time.Second))
	snap := d.Snapshot(detStart.Add(2 * time.Second))

	if snap.Harness != "claude" {
		t.Errorf("Harness = %q after inspect error, want claude retained", snap.Harness)
	}
}

// Byte observations flow through to the activity tracker: untainted
// output past the startup grace marks the identified harness working.
func TestDetector_OutputMarksWorking(t *testing.T) {
	ins := &fakeInspector{proc: ForegroundProc{PID: 4242, Comm: "claude", Argv: []string{"claude"}}}
	d := NewDetector(ins.inspect, detStart)
	d.Poll(detStart.Add(1 * time.Second))

	d.ObserveOutput(detStart.Add(5 * time.Second)) // well past grace
	snap := d.Snapshot(detStart.Add(5*time.Second + 500*time.Millisecond))

	if snap.State != StateWorking {
		t.Errorf("State = %q, want %q", snap.State, StateWorking)
	}
}

// Identifying a different harness (codex launched after claude exited)
// restarts the activity tracking, including its startup grace — the new
// harness's boot output must not inherit the old one's post-grace
// standing and read as working.
func TestDetector_ReidentificationResetsStartupGrace(t *testing.T) {
	ins := &fakeInspector{proc: ForegroundProc{PID: 4242, Comm: "claude", Argv: []string{"claude"}}}
	d := NewDetector(ins.inspect, detStart)
	d.Poll(detStart.Add(1 * time.Second))

	ins.proc = ForegroundProc{PID: 5000, Comm: "codex", Argv: []string{"codex"}}
	at := detStart.Add(10 * time.Second)
	d.Poll(at)
	d.ObserveOutput(at.Add(500 * time.Millisecond)) // codex boot noise, inside new grace
	snap := d.Snapshot(at.Add(700 * time.Millisecond))

	if snap.Harness != "codex" {
		t.Errorf("Harness = %q, want codex", snap.Harness)
	}
	if snap.State != StateIdle {
		t.Errorf("State = %q, want %q (boot output inside fresh grace)", snap.State, StateIdle)
	}
}
