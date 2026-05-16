#!/usr/bin/env bash
# `ppz diagnostics` surfaces daemon lifecycle events: `daemon_start`
# for the currently-running daemon, and `daemon_stop` for previous
# shutdowns. These are useful when investigating "did the daemon
# bounce while I was away?" without trawling through the daemon.log.
#
# The stop event from a previous daemon must persist across the
# restart — a fresh daemon's in-memory ring buffer is otherwise empty,
# so without on-disk persistence "the daemon stopped at HH:MM:SS"
# would vanish the instant the next daemon comes up. Test that this
# persistence works.
. /tests/lib/common.sh

HOME_D=/tmp/d-diag-lifecycle
rm -rf "$HOME_D"; mkdir -p "$HOME_D"
SOCK=$HOME_D/daemon.sock

cleanup() {
  PID=$(cat "$HOME_D/daemon.pid" 2>/dev/null || true)
  [[ -n "$PID" ]] && kill "$PID" 2>/dev/null || true
}
trap cleanup EXIT

# First daemon: start, then stop. The stop event must be persisted to
# disk so the second daemon below can observe it.
PPZ_HOME=$HOME_D PPZ_IPC_SOCKET=$SOCK ppz daemon start >/dev/null
PPZ_HOME=$HOME_D PPZ_IPC_SOCKET=$SOCK ppz daemon stop  >/dev/null

# Second daemon: start fresh. `ppz diagnostics` must list BOTH event
# types — the previous daemon_stop (loaded from the on-disk log) and
# the current daemon_start (just appended to the ring).
PPZ_HOME=$HOME_D PPZ_IPC_SOCKET=$SOCK ppz daemon start >/dev/null

PPZ_HOME=$HOME_D PPZ_IPC_SOCKET=$SOCK ppz diagnostics 2>/dev/null \
  | grep -oE "^(daemon_start|daemon_stop)" | sort -u
