#!/usr/bin/env bash
# Empty uncollared pipe → watch blocks until a message arrives. The NATS
# subject for an uncollared root pipe is "<acct>.<pipe>" (2 dot-parts),
# but the pre-fix subscribe callback used SplitN(subject, ".", 3) and
# required len==3, dropping uncollared publishes entirely.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1
ppz_a pipe create plaza >/dev/null

WATCH_OUT=/tmp/ls-watch-uc.out
rm -f "$WATCH_OUT"
ppz_a ls --watch > "$WATCH_OUT" 2>&1 &
WPID=$!
sleep 0.4  # let the NATS subscription set up before publishing the trigger

ppz_a send --from pubsub plaza "new uncollared" >/dev/null

# Watch should fire and exit. 10s budget.
for _ in $(seq 1 20); do
  if ! kill -0 "$WPID" 2>/dev/null; then break; fi
  sleep 0.5
done
wait "$WPID" 2>/dev/null || true

echo "--- watch fired on new uncollared message ---"
ls_normalize < "$WATCH_OUT"
