package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/daemon"
)

// cmdDaemonGroup dispatches `ppz daemon <subverb>` to start/stop/restart/login/logout.
func cmdDaemonGroup(args []string) error {
	if groupHelp("daemon", args) {
		return nil
	}
	if len(args) == 0 {
		printHelp(os.Stderr, "daemon")
		os.Exit(2)
	}
	switch args[0] {
	case "start":
		return cmdDaemonStart(args[1:])
	case "stop":
		return cmdDaemonStop(args[1:])
	case "restart":
		return cmdDaemonRestart(args[1:])
	case "login":
		return cmdDaemonLogin(args[1:])
	case "logout":
		return cmdDaemonLogout(args[1:])
	}
	fmt.Fprintf(os.Stderr, "ppz daemon: unknown subcommand %q\n", args[0])
	os.Exit(2)
	return nil
}

// cmdDaemonRestart: stop+start in one verb. The "daemon out of sync
// with ppz cli" state in `ppz status` recommends this — after an
// upgrade, the running daemon is still the previous binary until it's
// restarted, so a single command beats chaining `stop && start`.
//
// Output mirrors the two commands run in sequence: a "daemon stopped
// pid=PID" (or "daemon not running") line first, then "daemon started
// pid=PID" (or "daemon already running pid=PID" — though that's only
// possible if a race spawned a new one between stop and start).
//
// No flags. Returns the error from start; stop's failures (other than
// ESRCH-as-success) bubble up too.
func cmdDaemonRestart(args []string) error {
	if len(args) != 0 {
		fmt.Fprintln(os.Stderr, "usage: ppz daemon restart")
		os.Exit(2)
	}
	if err := cmdDaemonStop(nil); err != nil {
		return err
	}
	return cmdDaemonStart(nil)
}

// cmdDaemonStart:
//
//	--foreground : run the daemon in this process (used internally after fork
//	               and by docker compose).
//	(no flag)    : probe over IPC. If a daemon answers → print "daemon already
//	               running". Otherwise fork+exec self with `daemon start
//	               --foreground` in the background, wait for the socket to
//	               appear, print "daemon started".
func cmdDaemonStart(args []string) error {
	fs := flag.NewFlagSet("daemon start", flag.ExitOnError)
	foreground := fs.Bool("foreground", false, "run the daemon in the foreground")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *foreground {
		ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		d := daemon.New(home(), ipcSocket())
		return d.Run(ctx)
	}

	already, pid, err := ensureDaemonRunning()
	if err != nil {
		return err
	}
	if already {
		fmt.Fprintf(os.Stdout, "daemon already running pid=%d\n", pid)
	} else {
		fmt.Fprintf(os.Stdout, "daemon started pid=%d\n", pid)
	}
	return nil
}

// ensureDaemonRunning is idempotent: probes the IPC socket and returns
// (true, pid) if a daemon is already answering. Otherwise forks a new
// daemon, blocks until the socket comes up, and returns (false, pid).
//
// Used by `daemon start` (the explicit path) and by `daemon login` (the
// auto-start-on-first-use path) — login is the natural entry point and
// failing with E_DAEMON_NOT_RUNNING the first time someone runs it is
// hostile UX.
//
// Returns an error only on fork/wait failure; doesn't print anything
// itself so callers can shape their own messaging.
func ensureDaemonRunning() (already bool, pid int, err error) {
	var st cliproto.StatusReply
	probeErr := daemon.Call(ipcSocket(), cliproto.IPCStatus, struct{}{}, &st)
	already, pid, needFork := resolveDaemonPresence(st.DaemonPID, probeErr, livePIDFromHome())
	if !needFork {
		return already, pid, nil
	}
	p, err := forkDaemon()
	if err != nil {
		return false, 0, fmt.Errorf("start daemon: %w", err)
	}
	if err := waitForSocket(ipcSocket(), 5*time.Second); err != nil {
		return false, 0, fmt.Errorf("daemon did not come up: %w", err)
	}
	return false, p, nil
}

// resolveDaemonPresence decides, from the Status-probe result and the
// pidfile-owner's liveness, whether a daemon is already running (and its
// pid) or a fork is needed. Pure so it's unit-testable without a socket or
// a real fork; ensureDaemonRunning keeps the side effects.
func resolveDaemonPresence(probePID int, probeErr error, livePID int) (already bool, pid int, needFork bool) {
	if probeErr == nil && probePID != 0 {
		return true, probePID, false
	}
	// The Status probe didn't give a clean answer (transport error, or a
	// reply without a pid). Before forking — which would spawn a redundant
	// second daemon and rewrite the pidfile — check whether a live daemon
	// already owns the pidfile. A transient probe failure (a slow probe
	// right after logout, an IPC hiccup under load) must not duplicate a
	// daemon that's actually up; trust the pidfile + liveness over a single
	// flaky probe. A wedged-but-alive daemon then surfaces as a timeout on
	// the caller's real RPC rather than as a silent duplicate process.
	if livePID != 0 {
		return true, livePID, false
	}
	return false, 0, true
}

// livePIDFromHome returns the pid recorded in $PPZ_HOME/daemon.pid when that
// process is actually alive, else 0 (absent or stale pidfile).
func livePIDFromHome() int {
	pid := daemon.PIDFromHome(home())
	if daemon.IsAlive(pid) {
		return pid
	}
	return 0
}

// forkDaemon re-execs the current binary with `daemon start --foreground`,
// fully detached from this process. Stdio is redirected to a log file under
// $PPZ_HOME so the user has somewhere to look if it crashes.
func forkDaemon() (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(home(), 0o700); err != nil {
		return 0, err
	}
	logPath := filepath.Join(home(), "daemon.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	cmd := exec.Command(exe, "daemon", "start", "--foreground")
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(),
		"PPZ_HOME="+home(),
		"PPZ_IPC_SOCKET="+ipcSocket(),
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return 0, err
	}
	// Don't wait — let it run independently. Closing the parent's handle on
	// the log file is fine; the child holds its own.
	logFile.Close()
	// Capture the pid before scheduling Release: Release() sets Pid=-1
	// and on a loaded box the goroutine can race ahead of the return,
	// flipping the reported pid to -1 (caught in CI 2026-04-29).
	pid := cmd.Process.Pid
	go func() { _ = cmd.Process.Release() }()
	return pid, nil
}

// waitForSocket blocks until the socket file appears (and a bound listener
// accepts a Status RPC) or the timeout elapses.
func waitForSocket(sock string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var st cliproto.StatusReply
		if err := daemon.Call(sock, cliproto.IPCStatus, struct{}{}, &st); err == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errors.New("timed out waiting for daemon socket")
}
