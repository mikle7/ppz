#!/usr/bin/env bash
# `ppz daemon restart` against an empty $PPZ_HOME (no running daemon)
# behaves like `stop && start`: the stop half emits "daemon not running"
# (idempotent — see daemon-kill-idempotent), the start half forks a
# fresh daemon. Both halves succeed, exit 0.
#
# Pinned because the restart contract documented in WIRE.md depends on
# stop's ESRCH-as-success branch staying intact — a regression there
# would turn restart into a hard error from a clean machine.
. /tests/lib/common.sh

HOME_K=/tmp/k-restart-cold
rm -rf "$HOME_K"; mkdir -p "$HOME_K"
SOCK=$HOME_K/daemon.sock

cleanup() {
  PID=$(cat "$HOME_K/daemon.pid" 2>/dev/null || true)
  [[ -n "$PID" ]] && kill "$PID" 2>/dev/null || true
}
trap cleanup EXIT

PPZ_HOME=$HOME_K PPZ_IPC_SOCKET=$SOCK ppz daemon restart
