#!/usr/bin/env bash
# `ppz terminal watch` never advances the cursor — it's an observational
# UI, not a consumer. After view exits, ls still reports the channel as
# unread for that session.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal share term -- printf "msg" >/dev/null

# Wait for the .raw chunk to land. Use ls_normalize so the wait regex
# isn't sensitive to the table renderer's column padding.
wait_for 20 "ppz_a ls 2>/dev/null | ls_normalize | grep -E '^term\.stdout 1 ' >/dev/null" >/dev/null

# Run terminal view briefly. Should not advance the .raw cursor.
# We capture stdout to a file so we can poll for first-byte instead of
# blindly sleeping. Once view has emitted anything, it has subscribed
# and we can SIGINT cleanly.
WATCH_OUT=$(mktemp)
PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" ppz terminal watch term >"$WATCH_OUT" 2>&1 &
PID=$!
wait_for 20 "test -s '$WATCH_OUT'" >/dev/null
kill -INT "$PID" 2>/dev/null || true
wait "$PID" 2>/dev/null || true
rm -f "$WATCH_OUT"

# Cursor should still be 0 → unread = total = 1.
ppz_a ls | ls_normalize | grep '^term\.stdout'
