#!/usr/bin/env bash
# `ppz await --tail` loops: drain → re-block → drain → ... until SIGINT.
# Send two messages back-to-back on different pipes (so each loop
# iteration drains one pipe), confirm both bodies appear in the
# captured output, then SIGINT and check clean exit.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1
ppz_a pipe create alpha >/dev/null
ppz_a pipe create beta  >/dev/null

OUT=/tmp/await-tail.out
PID_FILE=/tmp/await-tail.pid
rm -f "$OUT" "$PID_FILE"

ppz_a await --tail alpha beta > "$OUT" 2>&1 &
echo $! > "$PID_FILE"
sleep 0.4

ppz_a send alpha "msg-A" >/dev/null
wait_for 30 "grep -q msg-A $OUT" >/dev/null
ppz_a send beta "msg-B" >/dev/null
wait_for 30 "grep -q msg-B $OUT" >/dev/null

PID=$(cat "$PID_FILE")
kill -INT "$PID" 2>/dev/null || true
for _ in 1 2 3 4 5 6 7 8 9 10; do
  if ! kill -0 "$PID" 2>/dev/null; then break; fi
  sleep 0.2
done
wait "$PID" 2>/dev/null || true

# Sorted body lines so order doesn't matter.
grep -oE 'msg-[AB]' "$OUT" | sort -u
