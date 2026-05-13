#!/usr/bin/env bash
# --tail drains retained messages, then keeps streaming new ones until
# SIGINT. Strategy: publish one message before launching `read --tail`
# in the background, publish another while the follower is up, then
# SIGINT and inspect what landed in the captured output.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create live >/dev/null
ppz_a send live.inbox "before" >/dev/null
wait_for 20 "ppz_a ls | grep -q before" >/dev/null

OUT=/tmp/follow.out
PID_FILE=/tmp/follow.pid
rm -f "$OUT" "$PID_FILE"

# Start the follower in the background. ppz_a is a shell function so we
# inline the env var that selects daemon-a.
PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" ppz read --bare live.inbox --tail >"$OUT" 2>&1 &
echo $! >"$PID_FILE"
# Wait for the follower to drain the retained "before" message before
# we publish "during" — that way "during" arrives via the live path,
# not as a retained replay (which would weaken the assertion).
wait_for 20 "grep -q before $OUT" >/dev/null

ppz_a send live.inbox "during" >/dev/null
wait_for 20 "grep -q during $OUT" >/dev/null

PID=$(cat "$PID_FILE")
kill -INT "$PID" 2>/dev/null || true
for _ in 1 2 3 4 5 6 7 8 9 10; do
  if ! kill -0 "$PID" 2>/dev/null; then break; fi
  sleep 0.1
done
wait "$PID" 2>/dev/null || true

cat "$OUT"
