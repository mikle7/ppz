#!/usr/bin/env bash
# RED → GREEN: external sends to <handle>.stdin must keep reaching the
# wrapped child after the share-side daemon is *logged out* and then
# *logged back in* — without a process restart.
#
# Bug: forwardStdin redials only when the IPC socket closes (the daemon
# process dies and SIGPIPEs the conn). `daemon logout` keeps the daemon
# alive, just clears credentials on disk; the watcher closes `d.NC` so
# the JetStream consumer goes silent — but the IPC conn stays open, so
# the CLI's `dec.Scan()` blocks forever. After re-login, no new
# consumer is built for the existing follow → bytes never land.
#
# Distinct from share-stdin-survives-share-daemon-restart, which
# exercises process-death recycle (which already redials on EOF).
. /tests/lib/common.sh

ppz_b daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

HOME_S=/tmp/share-logout
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

# Wrapped child: read lines, prefix with "got:", exit on QUIT.
ppz_s terminal share share-logout -- bash -c '
  while IFS= read -r line; do
    echo "got:$line"
    [ "$line" = QUIT ] && break
  done
' </dev/null >/dev/null 2>&1 &
SHARE_PID=$!

# Wait for the source's stdout pipe to provision.
wait_for 50 "ppz_s ls 2>/dev/null | grep -q '^share-logout.stdout'"

# Bob sends ONE; payload includes a trailing newline so bash's `read`
# returns it.
ppz_b send share-logout.stdin $'ONE\n' >/dev/null
wait_for 50 "ppz_s reread share-logout.stdout 2>/dev/null | grep -q 'got:ONE'"
echo "first_msg_received: $(ppz_s reread share-logout.stdout 2>/dev/null | grep -c 'got:ONE')"

# Recycle credentials WITHOUT killing the daemon process.
PID_BEFORE=$(cat "$HOME_S/daemon.pid")
ppz_s daemon logout >/dev/null
# Give the daemon's 50ms cred-poller time to react (drops d.NC).
sleep 0.3
ppz_s daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
PID_AFTER=$(cat "$HOME_S/daemon.pid")
[[ "$PID_BEFORE" = "$PID_AFTER" ]] && echo "daemon_same_pid: yes" || echo "daemon_same_pid: no"

# Bob sends TWO. Without the fix, the share's forwardStdin goroutine is
# still blocked reading from a dead consumer — TWO never reaches the
# PTY → never echoed → never on stdout.
ppz_b send share-logout.stdin $'TWO\n' >/dev/null
wait_for 50 "ppz_s reread share-logout.stdout 2>/dev/null | grep -q 'got:TWO'"
echo "second_msg_received: $(ppz_s reread share-logout.stdout 2>/dev/null | grep -c 'got:TWO')"

# Tell the wrapped bash to quit so the share exits cleanly.
ppz_b send share-logout.stdin $'QUIT\n' >/dev/null
wait_for 50 "! kill -0 $SHARE_PID 2>/dev/null"
