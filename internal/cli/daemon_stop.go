package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// cmdDaemonStop: alias for `kill $(cat $PPZ_HOME/daemon.pid)` with idempotent
// semantics. Sends SIGTERM, waits up to 5 s for the process to actually
// exit, then prints "daemon stopped pid=PID". If no daemon is running
// (no pid file, or the pid is stale), prints "daemon not running" exit 0
// — stop's job is "make sure it's stopped", and an already-stopped daemon
// already satisfies that.
func cmdDaemonStop(args []string) error {
	fs := flag.NewFlagSet("daemon stop", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	pidPath := filepath.Join(home(), "daemon.pid")
	data, err := os.ReadFile(pidPath)
	if err != nil {
		fmt.Fprintln(os.Stdout, "daemon not running")
		return nil
	}
	var pid int
	fmt.Sscanf(string(data), "%d", &pid)
	if pid <= 0 {
		fmt.Fprintln(os.Stdout, "daemon not running")
		return nil
	}
	if !processAlive(pid) {
		fmt.Fprintln(os.Stdout, "daemon not running")
		return nil
	}

	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		// ESRCH = "no such process": the daemon raced us and exited
		// already. Treat as success.
		if errors.Is(err, syscall.ESRCH) {
			fmt.Fprintln(os.Stdout, "daemon not running")
			return nil
		}
		return fmt.Errorf("kill %d: %w", pid, err)
	}

	// Wait for the process to actually exit. The daemon's deferred cleanup
	// (sock + pid file removal) needs to run, so a stale-state restart
	// doesn't catch a half-shutdown.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	fmt.Fprintf(os.Stdout, "daemon stopped pid=%d\n", pid)
	return nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	// Signal 0 is the canonical "is the pid live?" probe — no signal
	// delivered, just permission + existence checks.
	return syscall.Kill(pid, syscall.Signal(0)) == nil
}
