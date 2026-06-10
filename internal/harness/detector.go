package harness

import (
	"sync"
	"time"
)

// ForegroundProc is what the platform inspector reports about the PTY's
// foreground process group: the group leader's pid, its comm (binary
// name), and full argv (argv[0] included).
type ForegroundProc struct {
	PID  int
	Comm string
	Argv []string
}

// Detection is the point-in-time snapshot the heartbeat loop stamps
// into payloads. Zero value means "no harness in the foreground".
type Detection struct {
	Harness  string // canonical harness name, "" when none
	ChildPID int    // foreground pid when a harness is identified
	State    State  // StateUnknown when Harness is ""
}

// Detector owns harness detection for one wrapped PTY: it polls the
// foreground process via the injected inspector and folds byte
// observations into an ActivityTracker. Inspect is the platform seam —
// TIOCGPGRP + process lookup in production, a fake in tests.
//
// Inspect errors retain the previous identification (a transient ps
// failure must not flap `ppz who`); a successful inspect of a
// non-harness clears it. A harness change resets the activity tracker
// so each identification gets its own startup grace.
//
// Safe for concurrent use: the wrapper calls Observe* from its PTY I/O
// goroutines, Poll from the detection ticker, and Snapshot from the
// heartbeat loop.
type Detector struct {
	inspect func() (ForegroundProc, error)

	mu       sync.Mutex
	harness  string
	childPID int
	tracker  *ActivityTracker
}

// NewDetector creates a Detector for a PTY whose child spawned at
// `start`.
func NewDetector(inspect func() (ForegroundProc, error), start time.Time) *Detector {
	return &Detector{inspect: inspect}
}

// Poll re-inspects the foreground process and updates identification.
func (d *Detector) Poll(now time.Time) {
	proc, err := d.inspect()
	if err != nil {
		return // transient failure: retain the last identification
	}
	name := Identify(proc.Comm, proc.Argv)

	d.mu.Lock()
	defer d.mu.Unlock()
	if name == "" {
		d.harness, d.childPID, d.tracker = "", 0, nil
		return
	}
	if name != d.harness || proc.PID != d.childPID {
		// New harness (or a relaunch under a new pid): fresh tracker,
		// fresh startup grace.
		d.harness, d.childPID = name, proc.PID
		d.tracker = NewActivityTracker(now)
	}
}

// ObserveOutput forwards PTY output activity to the tracker.
func (d *Detector) ObserveOutput(now time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.tracker != nil {
		d.tracker.ObserveOutput(now)
	}
}

// ObserveInput forwards user input activity to the tracker.
func (d *Detector) ObserveInput(now time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.tracker != nil {
		d.tracker.ObserveInput(now)
	}
}

// Snapshot returns the current detection for heartbeat stamping.
func (d *Detector) Snapshot(now time.Time) Detection {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.harness == "" {
		return Detection{}
	}
	return Detection{Harness: d.harness, ChildPID: d.childPID, State: d.tracker.State(now)}
}
