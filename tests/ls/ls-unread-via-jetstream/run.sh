#!/usr/bin/env bash
# Three sends → ls shows total=3 unread=3 for the session that has
# never read. After `ppz read foo.inbox` from the same session, ls
# shows unread=0 (cursor advanced to the highest delivered seq).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create chat >/dev/null
ppz_a send chat.inbox "msg-1" >/dev/null
ppz_a send chat.inbox "msg-2" >/dev/null
ppz_a send chat.inbox "msg-3" >/dev/null
wait_for 20 "ppz_a ls 2>/dev/null | grep -q msg-3" >/dev/null

echo "--- before read ---"
ppz_a ls | grep '^chat\.inbox' | ls_normalize
ppz_a read chat.inbox >/dev/null
echo "--- after read ---"
ppz_a ls | grep '^chat\.inbox' | ls_normalize
