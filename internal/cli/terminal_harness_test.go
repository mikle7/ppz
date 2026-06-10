package cli

import (
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
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
