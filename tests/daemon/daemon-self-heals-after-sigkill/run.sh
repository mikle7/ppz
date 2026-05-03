#!/usr/bin/env bash
# After SIGKILL the daemon's deferred cleanup never runs, so the .sock and
# .pid files linger as orphans. The next `ppz daemon start` invocation must
# self-heal: detect the unbound socket via probe-fail, fork a new child, and
# the new child's startup must os.Remove the orphan socket and overwrite
# the orphan pid before binding.
#
# This is the safety net behind the claim that no manual rm is required
# after a hard kill.
. /tests/lib/common.sh

HOME_K=/tmp/k-heal
rm -rf "$HOME_K"; mkdir -p "$HOME_K"
SOCK=$HOME_K/daemon.sock

cleanup() {
  PID=$(cat "$HOME_K/daemon.pid" 2>/dev/null || true)
  [[ -n "$PID" ]] && kill "$PID" 2>/dev/null || true
}
trap cleanup EXIT

# Bring up the first daemon, then SIGKILL it (no defers run).
PPZ_HOME=$HOME_K PPZ_IPC_SOCKET=$SOCK ppz daemon start >/dev/null
PID1=$(cat "$HOME_K/daemon.pid")
kill -KILL "$PID1"
# Wait until the kernel has actually reaped the process.
while kill -0 "$PID1" 2>/dev/null; do sleep 0.05; done

# Confirm the orphans are present (i.e. cleanup truly didn't run).
[[ -e "$SOCK" ]] && echo "orphan-sock: yes" || echo "orphan-sock: no"
[[ -e "$HOME_K/daemon.pid" ]] && echo "orphan-pid: yes" || echo "orphan-pid: no"

# Re-launch — this is the self-heal under test.
PPZ_HOME=$HOME_K PPZ_IPC_SOCKET=$SOCK ppz daemon start
PPZ_HOME=$HOME_K PPZ_IPC_SOCKET=$SOCK ppz status | head -1
