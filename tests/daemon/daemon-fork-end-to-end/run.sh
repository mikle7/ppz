#!/usr/bin/env bash
# Smoke test for the dev-flow path. From a clean PPZ_HOME with NO pre-existing
# daemon, `ppz daemon start` must fork a working child; the daemon must reach NATS
# without any env-var coaching; and login -> create -> broadcast -> ls must
# round-trip a payload.
#
# This complements the daemon/daemon-already-running scenario (which tests the
# probe path against the compose-provisioned daemon) — without this scenario,
# bugs in the fork and auto-derivation paths sail past the suite.

. /tests/lib/common.sh

# Fresh, isolated PPZ_HOME for this scenario only. The compose daemon-a/b
# instances at /tmp/a and /tmp/b are untouched.
HOME_C=/tmp/c-fresh
rm -rf "$HOME_C"
mkdir -p "$HOME_C"
SOCK="$HOME_C/daemon.sock"

cleanup() {
  PID=$(cat "$HOME_C/daemon.pid" 2>/dev/null || true)
  [[ -n "$PID" ]] && kill "$PID" 2>/dev/null || true
}
trap cleanup EXIT

# Step 1: fork a fresh daemon (the bug we hit was that `ppz daemon start` without
# --foreground only probed instead of forking).
PPZ_HOME=$HOME_C PPZ_IPC_SOCKET=$SOCK ppz daemon start

# Step 2: login. NATS URL must auto-derive from the request's Host header
# (the bug we hit was the server hardcoding 'nats://ppz-server:4222' which
# host clients couldn't resolve).
PPZ_HOME=$HOME_C PPZ_IPC_SOCKET=$SOCK ppz daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

# Step 3: create -> broadcast -> ls.
PPZ_HOME=$HOME_C PPZ_IPC_SOCKET=$SOCK ppz source create fresh-flow >/dev/null
PPZ_HOME=$HOME_C PPZ_IPC_SOCKET=$SOCK ppz broadcast "it just works"
wait_for 20 "PPZ_HOME=$HOME_C PPZ_IPC_SOCKET=$SOCK ppz ls | grep -q 'it just works'" >/dev/null
PPZ_HOME=$HOME_C PPZ_IPC_SOCKET=$SOCK ppz ls | ls_normalize
