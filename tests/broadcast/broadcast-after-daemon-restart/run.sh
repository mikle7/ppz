#!/usr/bin/env bash
# Regression test. Reproduces the bug where, after `ppz daemon stop` followed by
# `ppz daemon start`, the daemon correctly reloads credentials from disk (so
# `status` and `ls` work), but it never re-establishes the NATS connection,
# so `broadcast` fails with E_NATS_UNREACHABLE.
#
# The fix is for the daemon to lazily exchange creds → NATS URL on the
# broadcast path when no NATS connection exists yet.
. /tests/lib/common.sh

HOME_K=/tmp/k-bcrestart
rm -rf "$HOME_K"; mkdir -p "$HOME_K"
SOCK=$HOME_K/daemon.sock

cleanup() {
  PID=$(cat "$HOME_K/daemon.pid" 2>/dev/null || true)
  [[ -n "$PID" ]] && kill "$PID" 2>/dev/null || true
}
trap cleanup EXIT

PPZ_HOME=$HOME_K PPZ_IPC_SOCKET=$SOCK ppz daemon start >/dev/null
PPZ_HOME=$HOME_K PPZ_IPC_SOCKET=$SOCK ppz daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
PPZ_HOME=$HOME_K PPZ_IPC_SOCKET=$SOCK ppz source create persists >/dev/null

# Restart cycle: ppz daemon stop (clean SIGTERM, defers run) + ppz daemon start (forks
# a fresh child that reloads creds from disk).
PPZ_HOME=$HOME_K PPZ_IPC_SOCKET=$SOCK ppz daemon stop   >/dev/null
PPZ_HOME=$HOME_K PPZ_IPC_SOCKET=$SOCK ppz daemon start >/dev/null

# Now broadcast must succeed without re-login.
PPZ_HOME=$HOME_K PPZ_IPC_SOCKET=$SOCK ppz broadcast "after restart"

# And the payload must reach the server's pipes table.
wait_for 20 "PPZ_HOME=$HOME_K PPZ_IPC_SOCKET=$SOCK ppz ls | grep -q 'after restart'" >/dev/null
PPZ_HOME=$HOME_K PPZ_IPC_SOCKET=$SOCK ppz ls | ls_normalize
