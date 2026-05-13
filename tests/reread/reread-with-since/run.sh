#!/usr/bin/env bash
# --since DURATION filters by created_at >= now - DURATION. Lives on
# `reread`. We send an "old" message, sleep long enough that it
# falls outside the window, then send a "new" message and reread
# with --since narrower than the gap.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a send chat.inbox "old" >/dev/null
sleep 1.2
ppz_a send chat.inbox "new" >/dev/null
wait_for 20 "ppz_a ls | grep -q '^chat.*new'" >/dev/null

ppz_a reread --bare chat.inbox --since 500ms
