#!/usr/bin/env bash
# `ppz ls --watch PATTERN` against an uncollared-only org should:
#   (a) include uncollared rows in the snapshot reply (today
#       buildFilteredList omits them entirely);
#   (b) match patterns against the bare pipe name when handle is empty.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1
ppz_a pipe create lobby-1 >/dev/null
ppz_a pipe create lobby-2 >/dev/null
ppz_a pipe create plaza >/dev/null
ppz_a send lobby-1 "lobby 1 msg" >/dev/null
ppz_a send plaza   "plaza msg"   >/dev/null
wait_for 20 "ppz_a ls 2>/dev/null | grep -q 'lobby 1 msg'" >/dev/null

# Pattern 'lobby-*' should match lobby-1 (unread) and lobby-2 (no
# unread) but NOT plaza.
echo "--- watch lobby-* — only matching uncollared rows ---"
ppz_a ls --watch 'lobby-*' | ls_normalize | sort
