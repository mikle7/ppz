#!/usr/bin/env bash
# `ppz terminal watch <handle>` streams the .stdout channel's bytes verbatim
# to its own stdout — no message-wrapping, no extra \n between chunks.
# After SIGINT it exits cleanly. The test runs view in the background,
# lets it drain, kills it, then strips the PTY-introduced \r so the
# remaining bytes match the wrapped child's logical output.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal share live -- sh -c 'echo first && echo second' >/dev/null

OUT=/tmp/view.out
PID_FILE=/tmp/view.pid
rm -f "$OUT" "$PID_FILE"

PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" ppz terminal watch live >"$OUT" 2>&1 &
echo $! >"$PID_FILE"
# Wait for view to drain — "second" is the last chunk emitted by the
# wrapped child, so once it shows up in OUT we know there's nothing
# more pending and SIGINT'ing now produces a stable transcript.
wait_for 20 "grep -q second $OUT" >/dev/null

PID=$(cat "$PID_FILE")
kill -INT "$PID" 2>/dev/null || true
for _ in 1 2 3 4 5 6 7 8 9 10; do
  kill -0 "$PID" 2>/dev/null || break
  sleep 0.1
done
wait "$PID" 2>/dev/null || true

# Strip \r (cooked-mode \n → \r\n) and the alt-screen / CSI wrapping
# bytes that view writes around the channel content. The dedicated
# `terminal-view-uses-alt-screen` test asserts the wrapping itself.
ESC=$(printf '\033')
tr -d '\r' < "$OUT" | sed -E "s/${ESC}\\[[?]?[0-9;]*[A-Za-z]//g"
