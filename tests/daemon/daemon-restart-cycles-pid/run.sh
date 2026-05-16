#!/usr/bin/env bash
# `ppz daemon restart` stops the running daemon and starts a fresh one.
# Equivalent to `ppz daemon stop && ppz daemon start` — provided so that
# operators don't have to chain commands after `ppz status` reports the
# red "daemon out of sync with ppz cli" state.
#
# Output: one "daemon stopped pid=PID" line then one "daemon started
# pid=PID" line, both at exit=0.
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
PPZ_HOME=$HOME_K PPZ_IPC_SOCKET=$SOCK ppz daemon restart
