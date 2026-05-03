#!/usr/bin/env bash
# `ppz daemon login` auto-starts the daemon if it isn't running. Without
# this, the canonical bootstrap is the awkward two-step
#   ppz daemon start && ppz daemon login URL -apikey K
# which agents and new users routinely get wrong (login fails with
# E_DAEMON_NOT_RUNNING and offers no in-line recovery). Login is the
# natural entry point — make it start the daemon for you.
. /tests/lib/common.sh

HOME_X=/tmp/login-autostart
rm -rf "$HOME_X"; mkdir -p "$HOME_X"
SOCK=$HOME_X/daemon.sock

cleanup() {
  PID=$(cat "$HOME_X/daemon.pid" 2>/dev/null || true)
  [[ -n "$PID" ]] && kill "$PID" 2>/dev/null || true
}
trap cleanup EXIT

# Daemon explicitly NOT running. Login should bring it up + complete.
PPZ_HOME=$HOME_X PPZ_IPC_SOCKET=$SOCK ppz daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)"

# Verify both side-effects happened in one call: daemon is up, creds
# stored.
echo "--- status after auto-start login ---"
PPZ_HOME=$HOME_X PPZ_IPC_SOCKET=$SOCK ppz status | grep -E '^(daemon|server):'
