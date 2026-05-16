#!/usr/bin/env bash
# Empty pipe → await blocks until a message arrives. Background the
# await, send concurrently, verify it unblocks within the timeout and
# drains the message.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1
ppz_a pipe create signal >/dev/null

OUT=/tmp/await-block.out
rm -f "$OUT"
ppz_a await signal > "$OUT" 2>&1 &
WPID=$!
sleep 0.4

ppz_a send signal "wake up" >/dev/null

for _ in $(seq 1 20); do
  if ! kill -0 "$WPID" 2>/dev/null; then break; fi
  sleep 0.5
done
wait "$WPID" 2>/dev/null || true

grep -E '^HH:MM:SS|wake up' "$OUT" | head -1 \
  | sed -E 's/^[0-9]{2}:[0-9]{2}:[0-9]{2}/HH:MM:SS/'
