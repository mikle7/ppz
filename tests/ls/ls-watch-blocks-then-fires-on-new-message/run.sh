#!/usr/bin/env bash
# When the session is caught up (no unread), `ppz ls --watch` blocks
# until a new message arrives on a matching pipe, then prints the
# snapshot and exits. The dominant agent loop:
#   while true; do ppz ls --watch | process; done
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create chat >/dev/null
ppz_a send chat.inbox "old" >/dev/null
wait_for 20 "ppz_a ls 2>/dev/null | grep -q old" >/dev/null
# Mark as read so unread = 0 — watch should NOT fire immediately.
ppz_a read chat.inbox >/dev/null

WATCH_OUT=/tmp/ls-watch.out
rm -f "$WATCH_OUT"
ppz_a ls --watch > "$WATCH_OUT" 2>&1 &
WPID=$!
# Give the daemon time to set up its NATS subscription before we publish;
# otherwise the trigger send races the consumer registration.
sleep 0.4

ppz_a send chat.inbox "new" >/dev/null

# Watch should now fire and exit. 10s budget covers any compose lag.
for _ in $(seq 1 20); do
  if ! kill -0 "$WPID" 2>/dev/null; then break; fi
  sleep 0.5
done
wait "$WPID" 2>/dev/null || true

echo "--- watch fired with new message visible ---"
ls_normalize < "$WATCH_OUT"
