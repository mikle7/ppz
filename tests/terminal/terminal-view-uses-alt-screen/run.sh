#!/usr/bin/env bash
# `ppz terminal watch` should behave like a proper TUI viewer: enter the
# alternate screen on start, restore on exit. That way the bytes it
# emits during the session — including any escape queries the wrapped
# child sent (DA1, focus, cursor-position) — never end up in the user's
# normal terminal scroll-back, and the post-view shell session is clean
# regardless of what the local terminal answered to those queries.
#
# We can't simulate a real local terminal in CI, but we CAN assert the
# alt-screen wrapping is present in view's stdout: enter sequence
# \x1b[?1049h near the start, exit sequence \x1b[?1049l near the end.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal share alt-test -- printf "hello" >/dev/null

OUT=/tmp/alt-test.out
PID_FILE=/tmp/alt-test.pid
rm -f "$OUT" "$PID_FILE"

PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" ppz terminal watch alt-test >"$OUT" 2>&1 &
echo $! >"$PID_FILE"
# Wait until view has written its initial frame (alt-screen enter +
# rendered content). Polling the output file is faster than the prior
# blanket sleep 1 — typical first byte lands in <100ms.
wait_for 20 "test -s '$OUT'" >/dev/null

PID=$(cat "$PID_FILE")
kill -INT "$PID" 2>/dev/null || true
for _ in 1 2 3 4 5 6 7 8 9 10; do
  kill -0 "$PID" 2>/dev/null || break
  sleep 0.1
done
wait "$PID" 2>/dev/null || true

# Look for the alt-screen wrapping bytes anywhere in the captured stream.
if grep -q $'\033\[?1049h' "$OUT"; then
  echo "alt-enter=yes"
else
  echo "alt-enter=no"
fi
if grep -q $'\033\[?1049l' "$OUT"; then
  echo "alt-exit=yes"
else
  echo "alt-exit=no"
fi
