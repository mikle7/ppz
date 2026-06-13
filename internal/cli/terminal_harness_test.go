package cli

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"

	"github.com/pipescloud/ppz/internal/harness"
)

// ptyForegroundInspector against a real PTY: spawn a child the same way
// cmdTerminalShare does (own session, controlling tty on the slave) and
// confirm TIOCGPGRP on the *master* fd resolves to the child's process
// group with a usable comm. This is the one production seam the harness
// unit tests can't fake — macOS in particular has a history of master-fd
// ioctls behaving differently from the slave side.
func TestPtyForegroundInspector_RealPTY(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skipf("pty open unavailable in this environment: %v", err)
	}
	defer ptmx.Close()

	cmd := exec.Command("sleep", "30")
	cmd.Stdin = tty
	cmd.Stdout = tty
	cmd.Stderr = tty
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		_ = tty.Close()
		t.Fatalf("start: %v", err)
	}
	_ = tty.Close()
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	inspect := ptyForegroundInspector(ptmx)

	// The foreground group is established by the kernel at setsid +
	// setctty time, but give a slow box a moment before failing.
	var lastErr error
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		proc, err := inspect()
		if err == nil {
			if proc.PID != cmd.Process.Pid {
				t.Fatalf("foreground pid = %d, want child %d", proc.PID, cmd.Process.Pid)
			}
			if proc.Comm != "sleep" {
				t.Fatalf("comm = %q, want sleep", proc.Comm)
			}
			return
		}
		lastErr = err
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("inspect never succeeded: %v", lastErr)
}

// countingInspector is a settable foreground-process source standing in
// for the TIOCGPGRP seam, counting inspections so the poll-cadence
// tests can assert how often the loop actually forked a lookup.
type countingInspector struct {
	mu    sync.Mutex
	calls int
	proc  harness.ForegroundProc
	err   error
}

func (c *countingInspector) inspect() (harness.ForegroundProc, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	return c.proc, c.err
}

func (c *countingInspector) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// waitForInspections blocks until the inspector has been called `want`
// times (the loop body runs asynchronously after a tick is received),
// then settles briefly and fails on overshoot — the cadence tests care
// about "exactly N polls", not "at least N".
func waitForInspections(t *testing.T, c *countingInspector, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if c.count() >= want {
			time.Sleep(50 * time.Millisecond)
			if got := c.count(); got != want {
				t.Fatalf("inspector calls = %d, want exactly %d", got, want)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("inspector calls = %d, want %d", c.count(), want)
}

// During the acquisition window every snapshot tick also re-inspects
// the foreground — the user's command is booting and the harness can
// appear at any moment.
func TestRunHarnessDetection_AcquisitionWindowPollsEveryTick(t *testing.T) {
	ins := &countingInspector{proc: harness.ForegroundProc{PID: 1, Comm: "zsh", Argv: []string{"zsh"}}}
	det := harness.NewDetector(ins.inspect)
	started := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	tick := make(chan time.Time)
	wake := make(chan struct{}, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runHarnessDetection(ctx, det, started, wake, tick)

	tick <- started.Add(1 * time.Second)
	tick <- started.Add(2 * time.Second)
	tick <- started.Add(3 * time.Second)

	waitForInspections(t, ins, 3)
}

// Past the acquisition window, inspection relaxes to the steady
// interval — on darwin every poll forks ps, so the loop must not
// re-inspect on every 1s snapshot tick forever. Snapshot ticks between
// polls still run (state transitions don't depend on inspection), they
// just skip the process lookup.
func TestRunHarnessDetection_SteadyStateGatesPollsToSteadyInterval(t *testing.T) {
	ins := &countingInspector{proc: harness.ForegroundProc{PID: 1, Comm: "zsh", Argv: []string{"zsh"}}}
	det := harness.NewDetector(ins.inspect)
	started := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	tick := make(chan time.Time)
	wake := make(chan struct{}, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runHarnessDetection(ctx, det, started, wake, tick)

	tick <- started.Add(11 * time.Second) // first ever poll
	tick <- started.Add(12 * time.Second) // 1s since last poll: skipped
	tick <- started.Add(13 * time.Second) // 2s: skipped
	tick <- started.Add(16 * time.Second) // 5s: polls again

	waitForInspections(t, ins, 2)
}

// The loop nudges the wake channel when the detection snapshot changes
// — identification appearing, and later the working transition — and
// stays quiet across ticks where nothing changed, so the heartbeat
// only emits out-of-cycle beats for real transitions.
func TestRunHarnessDetection_WakesOnSnapshotChangeOnly(t *testing.T) {
	ins := &countingInspector{proc: harness.ForegroundProc{PID: 4242, Comm: "claude", Argv: []string{"claude"}}}
	det := harness.NewDetector(ins.inspect)
	started := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	tick := make(chan time.Time)
	wake := make(chan struct{}, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runHarnessDetection(ctx, det, started, wake, tick)

	tick <- started.Add(1 * time.Second) // identifies claude → snapshot changed
	select {
	case <-wake:
	case <-time.After(2 * time.Second):
		t.Fatal("no wake after harness identification")
	}

	tick <- started.Add(2 * time.Second) // identified, still idle: no change
	tick <- started.Add(3 * time.Second) // receipt proves the 2s body completed
	waitForInspections(t, ins, 3)
	select {
	case <-wake:
		t.Fatal("wake fired for an unchanged snapshot")
	default:
	}

	// Output past the startup grace flips idle→working; the next
	// snapshot tick must wake the heartbeat.
	det.ObserveOutput(started.Add(6 * time.Second))
	tick <- started.Add(6*time.Second + 100*time.Millisecond)
	select {
	case <-wake:
	case <-time.After(2 * time.Second):
		t.Fatal("no wake after idle→working transition")
	}
}

// identifiedDetector returns a Detector that already identified a
// claude harness long enough ago that the startup grace has lapsed, so
// observer tests exercise pure causality.
func identifiedDetector() *harness.Detector {
	proc := harness.ForegroundProc{PID: 4242, Comm: "claude", Argv: []string{"claude"}}
	d := harness.NewDetector(func() (harness.ForegroundProc, error) { return proc, nil })
	d.Poll(time.Now().Add(-time.Minute))
	return d
}

// PTY output read through the tee marks the identified harness working
// — this is the only place output causality enters the detector, so
// the tee must both pass the bytes through and stamp the observation.
func TestHarnessOutputReader_ReadMarksWorking(t *testing.T) {
	det := identifiedDetector()
	r := harnessOutputReader{r: strings.NewReader("agent output"), det: det}

	buf := make([]byte, 32)
	n, err := r.Read(buf)
	if err != nil || string(buf[:n]) != "agent output" {
		t.Fatalf("Read = (%q, %v), want passthrough", buf[:n], err)
	}
	if got := det.Snapshot(time.Now()).State; got != harness.StateWorking {
		t.Errorf("state after untainted read = %q, want %q", got, harness.StateWorking)
	}
}

// A zero-byte read (EOF, the child exiting) carries no agent activity
// and must not refresh the working window.
func TestHarnessOutputReader_EmptyReadDoesNotMarkWorking(t *testing.T) {
	det := identifiedDetector()
	r := harnessOutputReader{r: strings.NewReader(""), det: det}

	_, _ = r.Read(make([]byte, 32))
	if got := det.Snapshot(time.Now()).State; got != harness.StateIdle {
		t.Errorf("state after empty read = %q, want %q", got, harness.StateIdle)
	}
}

// Input injected through the tee (remote .stdin, alert submission)
// taints causality before the bytes reach the PTY: the echo that
// follows immediately must read as idle, exactly like a local
// keystroke's echo.
func TestHarnessInputWriter_TaintsSubsequentOutputAsEcho(t *testing.T) {
	det := identifiedDetector()
	var sink bytes.Buffer
	w := harnessInputWriter{&sink, det}

	if _, err := w.Write([]byte("injected")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if sink.String() != "injected" {
		t.Fatalf("passthrough = %q, want %q", sink.String(), "injected")
	}

	det.ObserveOutput(time.Now()) // the injected bytes' echo
	if got := det.Snapshot(time.Now()).State; got != harness.StateIdle {
		t.Errorf("echo after injected input = %q, want %q (tainted)", got, harness.StateIdle)
	}
}

// With a screen writer wired, the output tee feeds the same bytes into
// the live screen model that it passes through to the display/publish
// path — this is the only place the screen model learns what the child
// drew, so blocked detection sees exactly what the user sees.
func TestHarnessOutputReader_FeedsScreen(t *testing.T) {
	det := identifiedDetector()
	var screen bytes.Buffer
	r := harnessOutputReader{r: strings.NewReader("dialog bytes"), det: det, screen: &screen}

	buf := make([]byte, 32)
	n, err := r.Read(buf)
	if err != nil || string(buf[:n]) != "dialog bytes" {
		t.Fatalf("Read = (%q, %v), want passthrough", buf[:n], err)
	}
	if screen.String() != "dialog bytes" {
		t.Errorf("screen received %q, want %q", screen.String(), "dialog bytes")
	}
}
