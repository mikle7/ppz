#!/usr/bin/env bash
# RED → GREEN: external sends to <handle>.stdin must keep reaching the
# wrapped child after the share-side daemon is recycled.
#
# Bug: forwardStdin opens one IPC connection at share-start. When the
# daemon stops, scanner returns EOF, the goroutine exits — and never
# reconnects. Subsequent `ppz send <handle>.stdin` calls reach NATS
# fine but the share has no live subscriber, so the bytes never land
# on the PTY master.
#
# We spin up an ad-hoc share-side daemon in the test-runner container
# (a fork we can actually SIGTERM) and use ppz_b for the sender side.
. /tests/lib/common.sh

ppz_b daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

HOME_S=/tmp/share-restart
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
ppz_s terminal share share-restart -- bash -c '
  while IFS= read -r line; do
    echo "got:$line"
    [ "$line" = QUIT ] && break
  done
' </dev/null >/dev/null 2>&1 &
SHARE_PID=$!

# Wait for the source's stdout pipe to provision.
wait_for 50 "ppz_s ls 2>/dev/null | grep -q '^share-restart.stdout'"

# Bob sends ONE; payload includes a trailing newline so bash's `read`
# returns it.
ppz_b send share-restart.stdin $'ONE\n' >/dev/null
wait_for 50 "ppz_s reread share-restart.stdout 2>/dev/null | grep -q 'got:ONE'"
echo "first_msg_received: $(ppz_s reread share-restart.stdout 2>/dev/null | grep -c 'got:ONE')"

# Recycle the share-side daemon. Forks child that reloads creds from
# disk and listens on the same socket again.
PID_BEFORE=$(cat "$HOME_S/daemon.pid")
ppz_s daemon stop  >/dev/null
ppz_s daemon start >/dev/null
PID_AFTER=$(cat "$HOME_S/daemon.pid")
[[ "$PID_BEFORE" != "$PID_AFTER" ]] && echo "daemon_recycled: yes" || echo "daemon_recycled: no"

# Bob sends TWO. Without the fix, the share's forwardStdin goroutine
# died on the first daemon stop and never reconnected — TWO never
# reaches the PTY → never echoed → never on stdout.
ppz_b send share-restart.stdin $'TWO\n' >/dev/null
wait_for 50 "ppz_s reread share-restart.stdout 2>/dev/null | grep -q 'got:TWO'"
echo "second_msg_received: $(ppz_s reread share-restart.stdout 2>/dev/null | grep -c 'got:TWO')"

# Tell the wrapped bash to quit so the share exits cleanly.
ppz_b send share-restart.stdin $'QUIT\n' >/dev/null
wait_for 50 "! kill -0 $SHARE_PID 2>/dev/null"
