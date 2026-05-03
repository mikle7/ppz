#!/usr/bin/env bash
# Two consecutive `ppz daemon stop` invocations both succeed (exit 0). The second
# is a no-op — already stopped. Output:
#   1st: daemon stopped pid=PID
#   2nd: daemon not running
. /tests/lib/common.sh

HOME_K=/tmp/k-idem
rm -rf "$HOME_K"; mkdir -p "$HOME_K"
SOCK=$HOME_K/daemon.sock

cleanup() {
  PID=$(cat "$HOME_K/daemon.pid" 2>/dev/null || true)
  [[ -n "$PID" ]] && kill "$PID" 2>/dev/null || true
}
trap cleanup EXIT

PPZ_HOME=$HOME_K PPZ_IPC_SOCKET=$SOCK ppz daemon start >/dev/null
PPZ_HOME=$HOME_K PPZ_IPC_SOCKET=$SOCK ppz daemon stop
PPZ_HOME=$HOME_K PPZ_IPC_SOCKET=$SOCK ppz daemon stop
