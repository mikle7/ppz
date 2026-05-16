#!/usr/bin/env bash
# `ppz daemon restart` stops the running daemon and starts a fresh one.
# Equivalent to `ppz daemon stop && ppz daemon start` — provided so that
# operators don't have to chain commands after `ppz status` reports the
# red "daemon out of sync with ppz cli" state.
#
# Asserts the PID actually changes (otherwise the verb could
# silently no-op and we'd still see two normalized "pid=PID" lines).
. /tests/lib/common.sh

HOME_K=/tmp/k-restart
rm -rf "$HOME_K"; mkdir -p "$HOME_K"
SOCK=$HOME_K/daemon.sock

cleanup() {
  PID=$(cat "$HOME_K/daemon.pid" 2>/dev/null || true)
  [[ -n "$PID" ]] && kill "$PID" 2>/dev/null || true
}
trap cleanup EXIT

PPZ_HOME=$HOME_K PPZ_IPC_SOCKET=$SOCK ppz daemon start >/dev/null
PID_BEFORE=$(cat "$HOME_K/daemon.pid")
PPZ_HOME=$HOME_K PPZ_IPC_SOCKET=$SOCK ppz daemon restart
PID_AFTER=$(cat "$HOME_K/daemon.pid")
if [[ "$PID_BEFORE" != "$PID_AFTER" ]]; then
  echo "pid-cycled: yes"
else
  echo "pid-cycled: no"
fi
