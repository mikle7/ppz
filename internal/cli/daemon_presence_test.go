package cli

import (
	"errors"
	"testing"
)

// resolveDaemonPresence is the fork/no-fork decision behind
// ensureDaemonRunning. The flaky e2e
// terminal/share-inbox-alerts-survives-share-daemon-logout caught the bug it
// guards: a transient Status-probe failure (slow probe right after logout, an
// IPC hiccup under CI load) made `ppz daemon login` fork a SECOND daemon,
// changing the pidfile pid even though the original daemon was still alive.

// Happy path: the Status probe answered with a pid → that daemon is running,
// no fork.
func TestResolveDaemonPresence_ProbeOK_NoFork(t *testing.T) {
	already, pid, needFork := resolveDaemonPresence(4321, nil, 0)
	if needFork {
		t.Errorf("a clean Status reply must not fork")
	}
	if !already || pid != 4321 {
		t.Errorf("want already=true pid=4321, got already=%v pid=%d", already, pid)
	}
}

// The regression: the Status probe failed, but a live daemon already owns the
// pidfile. Forking here spawns a redundant second daemon and rewrites the
// pidfile (the e2e's `daemon_same_pid: no`). Must NOT fork; report the live
// daemon as already running.
func TestResolveDaemonPresence_ProbeFailsButDaemonAlive_NoFork(t *testing.T) {
	already, pid, needFork := resolveDaemonPresence(0, errors.New("ipc: read timeout"), 1234)
	if needFork {
		t.Errorf("a live daemon owns the pidfile (1234); a transient Status error must not fork a duplicate")
	}
	if !already || pid != 1234 {
		t.Errorf("want already=true pid=1234 (the live daemon), got already=%v pid=%d", already, pid)
	}
}

// No live daemon and the probe failed → genuinely not running, so fork. This
// is the legitimate auto-start path that must be preserved.
func TestResolveDaemonPresence_ProbeFailsNoDaemon_Forks(t *testing.T) {
	already, pid, needFork := resolveDaemonPresence(0, errors.New("dial: connection refused"), 0)
	if !needFork {
		t.Errorf("no live daemon + failed probe must fork to auto-start")
	}
	if already || pid != 0 {
		t.Errorf("want already=false pid=0 before the fork, got already=%v pid=%d", already, pid)
	}
}

// Defensive: a probe that returns no error but a zero pid (a daemon that
// somehow replied without its pid) is treated as "no clean answer" — fall
// through to the liveness check rather than trusting pid 0.
func TestResolveDaemonPresence_ProbeZeroPIDButDaemonAlive_NoFork(t *testing.T) {
	_, pid, needFork := resolveDaemonPresence(0, nil, 1234)
	if needFork {
		t.Errorf("zero probe pid with a live pidfile must not fork")
	}
	if pid != 1234 {
		t.Errorf("want pid=1234 from the live pidfile, got %d", pid)
	}
}
