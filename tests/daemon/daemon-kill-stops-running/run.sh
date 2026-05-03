#!/usr/bin/env bash
# `ppz daemon stop` against a running daemon: prints "daemon stopped pid=PID" exit 0,
# and a follow-up `ppz status` then reports "daemon: not running" (exit 11).
. /tests/lib/common.sh

HOME_K=/tmp/k-stops
rm -rf "$HOME_K"; mkdir -p "$HOME_K"
SOCK=$HOME_K/daemon.sock

cleanup() {
  PID=$(cat "$HOME_K/daemon.pid" 2>/dev/null || true)
  [[ -n "$PID" ]] && kill "$PID" 2>/dev/null || true
}
trap cleanup EXIT

PPZ_HOME=$HOME_K PPZ_IPC_SOCKET=$SOCK ppz daemon start >/dev/null
PPZ_HOME=$HOME_K PPZ_IPC_SOCKET=$SOCK ppz daemon stop
PPZ_HOME=$HOME_K PPZ_IPC_SOCKET=$SOCK ppz status
