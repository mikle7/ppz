#!/usr/bin/env bash
# Cursors are keyed by PPZ_SESSION. Two sessions on the same daemon have
# independent unread counts on the same channel.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create chat >/dev/null
ppz_a send chat.inbox "only-msg" >/dev/null
wait_for 20 "ppz_a ls 2>/dev/null | grep -q only-msg" >/dev/null

# Agent A reads (cursor for session 'agent-a' advances).
PPZ_SESSION=agent-a ppz_a read chat.inbox >/dev/null

echo "--- agent-a ls ---"
PPZ_SESSION=agent-a ppz_a ls | grep '^chat\.inbox' | ls_normalize

echo "--- agent-b ls ---"
PPZ_SESSION=agent-b ppz_a ls | grep '^chat\.inbox' | ls_normalize
