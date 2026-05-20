#!/usr/bin/env bash
# End-to-end: another client `ppz send`s to <handle>.stdin → the terminal
# wrapper writes those bytes to the PTY master → the child reads them →
# whatever it prints lands on <handle>.stdout. We use `cat` with TTY echo
# disabled so stdout reflects only the child's emission of what it read.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

# Run the cat-loop in the background. exec replaces sh with cat.
PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" ppz terminal share echo-pipe -- \
  sh -c 'stty -echo 2>/dev/null; exec cat' >/dev/null 2>&1 &
TERM_PID=$!
# Wait for the source to land in the daemon's view before we send. The
# blanket `sleep 1` was a guess at how long `terminal share` needs to
# allocate the pty + register the source; polling `ls` cuts that to
# whatever it actually takes (typically <200ms).
wait_for 20 "ppz_a ls 2>/dev/null | ls_normalize | grep -q '^echo-pipe.stdin'" >/dev/null

ppz_a send --from pubsub echo-pipe.stdin $'hello-from-send\n' >/dev/null
wait_for 20 "ppz_a reread echo-pipe.stdout | grep -q hello-from-send" >/dev/null

kill "$TERM_PID" 2>/dev/null || true
wait "$TERM_PID" 2>/dev/null || true

# .stdout is byte-faithful (was line-segmented), so OPOST + Fprintln add
# extra \r and \n bytes whose exact count varies by platform PTY (macOS
# Docker Desktop vs Linux runners differ). Squash to a single line per
# non-empty content — the assertion is "the message round-tripped".
ppz_a reread echo-pipe.stdout | tr -d '\r' | sed '/^$/d'
