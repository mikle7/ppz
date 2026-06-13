package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/harness"
)

// This file wires internal/harness into the terminal-share wrapper:
// the platform inspector (which process is in the PTY's foreground),
// the byte-flow observers (output/input causality), and the detection
// poll loop that wakes the heartbeat on state transitions. See
// docs/specs/agent-detection.md.

// ptyForegroundInspector is the production seam for harness.Detector:
// it reads the PTY's foreground process group via TIOCGPGRP on the
// master fd, then resolves the group leader's comm and argv.
func ptyForegroundInspector(ptmx *os.File) func() (harness.ForegroundProc, error) {
	return func() (harness.ForegroundProc, error) {
		pgid, err := unix.IoctlGetInt(int(ptmx.Fd()), unix.TIOCGPGRP)
		if err != nil {
			return harness.ForegroundProc{}, fmt.Errorf("tiocgpgrp: %w", err)
		}
		comm, argv, err := processCommArgv(pgid)
		if err != nil {
			return harness.ForegroundProc{}, err
		}
		return harness.ForegroundProc{PID: pgid, Comm: comm, Argv: argv}, nil
	}
}

// processCommArgv resolves a pid to its binary name and argv: /proc on
// linux, ps(1) on darwin. The darwin argv is whitespace-split (ps can't
// preserve quoting), which is fine for detection — Identify scans
// token basenames, it doesn't re-execute anything.
func processCommArgv(pid int) (string, []string, error) {
	if runtime.GOOS == "linux" {
		comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
		if err != nil {
			return "", nil, err
		}
		raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err != nil {
			return "", nil, err
		}
		argv := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
		return strings.TrimSpace(string(comm)), argv, nil
	}
	comm, err := exec.Command("ps", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", nil, err
	}
	args, err := exec.Command("ps", "-o", "args=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", nil, err
	}
	return filepath.Base(strings.TrimSpace(string(comm))), strings.Fields(strings.TrimSpace(string(args))), nil
}

// Detection poll cadence: tight while the user's command is booting
// (the harness usually appears within a few seconds of spawn), relaxed
// once the session has settled. Process inspection forks ps on darwin,
// so the steady-state interval stays coarse; state transitions don't
// depend on it — they come from the byte observers and are re-checked
// every snapshotInterval.
const (
	detectAcquisitionWindow   = 10 * time.Second
	detectAcquisitionInterval = time.Second
	detectSteadyInterval      = 5 * time.Second
	detectSnapshotInterval    = time.Second
)

// harnessScreenBottomLines is how much of the live screen the blocked
// detectors see. Dialogs and prompts render at the bottom; 50 lines
// matches what herdr found sufficient across harness UIs.
const harnessScreenBottomLines = 50

// syncScreenSize mirrors the child PTY's current geometry into the
// live screen model so it always renders at the child's real size.
// Errors keep the previous grid — a stale size degrades pattern
// matching, it must never break the share.
func syncScreenSize(screen *cliproto.LiveScreen, ptmx *os.File) {
	ws, err := pty.GetsizeFull(ptmx)
	if err != nil || ws.Cols == 0 || ws.Rows == 0 {
		return
	}
	screen.Resize(int(ws.Cols), int(ws.Rows))
}

// runHarnessDetection polls the detector and nudges `wake` whenever the
// detection snapshot changes (harness appeared/exited, working↔idle),
// so the heartbeat loop emits an immediate out-of-cycle beat instead of
// `ppz who` waiting out the 60s tick. The send is non-blocking against
// a 1-buffered channel: bursts coalesce into one wake. The caller owns
// the snapshot ticker and passes its channel (production ticks every
// detectSnapshotInterval; tests inject times directly), same seam as
// heartbeatDeps.Tick.
func runHarnessDetection(ctx context.Context, det *harness.Detector, started time.Time, wake chan<- struct{}, tick <-chan time.Time) {
	var last harness.Detection
	var lastPoll time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick:
			interval := detectSteadyInterval
			if now.Sub(started) < detectAcquisitionWindow {
				interval = detectAcquisitionInterval
			}
			if lastPoll.IsZero() || now.Sub(lastPoll) >= interval {
				det.Poll(now)
				lastPoll = now
			}
			snap := det.Snapshot(now)
			if snap != last {
				last = snap
				select {
				case wake <- struct{}{}:
				default:
				}
			}
		}
	}
}

// harnessOutputReader tees read timestamps into the detector so PTY
// output counts as agent activity, and (when screen is non-nil) feeds
// the same bytes into the live screen model for blocked-state
// detection. Data passes through untouched either way.
type harnessOutputReader struct {
	r      io.Reader
	det    *harness.Detector
	screen io.Writer // optional live screen model; nil disables
}

func (h harnessOutputReader) Read(p []byte) (int, error) {
	n, err := h.r.Read(p)
	if n > 0 {
		h.det.ObserveOutput(time.Now())
		if h.screen != nil {
			_, _ = h.screen.Write(p[:n])
		}
	}
	return n, err
}

// harnessInputWriter tees write timestamps into the detector so
// injected input (remote .stdin messages) taints output causality the
// same way local keystrokes do. Data passes through untouched.
type harnessInputWriter struct {
	w   io.Writer
	det *harness.Detector
}

func (h harnessInputWriter) Write(p []byte) (int, error) {
	if len(p) > 0 {
		h.det.ObserveInput(time.Now())
	}
	return h.w.Write(p)
}
