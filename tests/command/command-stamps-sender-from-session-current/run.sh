#!/usr/bin/env bash
# `ppz command <handle>` publishes to <handle>.stdin and shares the
# session-id-forwarding contract with `ppz send`. Pre-fix the CLI
# constructed BroadcastRequest without Session, so the daemon stamped
# sender="" on every command instruction. This fixture pins sender
# coverage for the command path so a regression won't slip in silently.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

# Run a pty-backed cat so `agent.stdin` exists. terminal share auto-
# creates the source as kind=pty (which carries stdin/stdout pipes).
PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" ppz terminal share agent -- \
  sh -c 'stty -echo 2>/dev/null; exec cat' >/dev/null 2>&1 &
TERM_PID=$!
wait_for 20 "ppz_a ls 2>/dev/null | ls_normalize | grep -q '^agent.stdin'" >/dev/null

# Shift session current to a different handle so sender ≠ destination.
# (Source create on a non-pty handle sets current; agent stays kind=pty
# so sources differ — the test discriminates the two.)
ppz_a source create operator >/dev/null

# `--none` suppresses the trailing control sequence so only one envelope
# lands on agent.stdin — keeps the assertion deterministic.
ppz_a command agent "ls -la" --none >/dev/null
wait_for 20 "ppz_a reread agent.stdin --json | jq -e '.payload | length > 0' >/dev/null" >/dev/null

kill "$TERM_PID" 2>/dev/null || true
wait "$TERM_PID" 2>/dev/null || true

# Project sender + payload from the latest envelope on agent.stdin.
# Expected: sender=operator (publishing session's current source),
# payload="ls -la" (the instruction).
ppz_a reread agent.stdin --json | jq -c '{sender, payload}'
