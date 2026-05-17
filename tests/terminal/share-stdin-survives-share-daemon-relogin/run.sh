#!/usr/bin/env bash
# RED → GREEN: external sends to <handle>.stdin must keep reaching the
# wrapped child after the share-side daemon is *logged in again* on
# top of an existing logged-in session.
#
# Why this differs from -logout and -restart:
#   - -restart   exercises process death + redial on EOF (already
#                 works).
#   - -logout    exercises the watcher closing d.NC when credentials
#                 disappear, while the daemon process keeps running.
#   - -relogin   exercises handleLogin's own NC swap
#                 (handlers.go: d.NC.Close(); d.NC = nc). Same NC-swap
#                 shape as a credential rotation in the refresh loop,
#                 without needing to wait for a JWT expiry — re-running
#                 login is a deterministic way to force the swap from
#                 a test.
#
# Both -logout and -relogin should be made green by the same
# centralized swapNC helper that closes all registered follow conns
# before replacing d.NC.
. /tests/lib/common.sh

ppz_b daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

HOME_S=/tmp/share-relogin
rm -rf "$HOME_S"; mkdir -p "$HOME_S"
SOCK_S=$HOME_S/daemon.sock
ppz_s() { PPZ_HOME=$HOME_S PPZ_IPC_SOCKET=$SOCK_S ppz "$@"; }

cleanup() {
  PID=$(cat "$HOME_S/daemon.pid" 2>/dev/null || true)
  [[ -n "$PID" ]] && kill "$PID" 2>/dev/null || true
  kill "$SHARE_PID" 2>/dev/null || true
  wait "$SHARE_PID" 2>/dev/null || true
}
trap cleanup EXIT

ppz_s daemon start >/dev/null
ppz_s daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

ppz_s terminal share share-relogin -- bash -c '
  while IFS= read -r line; do
    echo "got:$line"
    [ "$line" = QUIT ] && break
  done
' </dev/null >/dev/null 2>&1 &
SHARE_PID=$!

wait_for 50 "ppz_s ls 2>/dev/null | grep -q '^share-relogin.stdout'"

ppz_b send share-relogin.stdin $'ONE\n' >/dev/null
wait_for 50 "ppz_s reread share-relogin.stdout 2>/dev/null | grep -q 'got:ONE'"
echo "first_msg_received: $(ppz_s reread share-relogin.stdout 2>/dev/null | grep -c 'got:ONE')"

# Re-run login WITHOUT logout. The daemon's handleLogin tears the
# existing d.NC down and swaps in a fresh one — old NC-anchored
# consumers (including the follow on .stdin) go silent.
PID_BEFORE=$(cat "$HOME_S/daemon.pid")
ppz_s daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
PID_AFTER=$(cat "$HOME_S/daemon.pid")
[[ "$PID_BEFORE" = "$PID_AFTER" ]] && echo "daemon_same_pid: yes" || echo "daemon_same_pid: no"

ppz_b send share-relogin.stdin $'TWO\n' >/dev/null
wait_for 50 "ppz_s reread share-relogin.stdout 2>/dev/null | grep -q 'got:TWO'"
echo "second_msg_received: $(ppz_s reread share-relogin.stdout 2>/dev/null | grep -c 'got:TWO')"

ppz_b send share-relogin.stdin $'QUIT\n' >/dev/null
wait_for 50 "! kill -0 $SHARE_PID 2>/dev/null"
