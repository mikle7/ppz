#!/usr/bin/env bash
# Three broadcasts → ls shows total=3 unread=3 for the session that has
# never read. After `ppz read foo.broadcast` from the same session, ls
# shows unread=0 (cursor advanced to the highest delivered seq).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a broadcast -m "msg-1" >/dev/null
ppz_a broadcast -m "msg-2" >/dev/null
ppz_a broadcast -m "msg-3" >/dev/null
wait_for 20 "ppz_a ls 2>/dev/null | grep -q msg-3" >/dev/null

echo "--- before read ---"
ppz_a ls | grep '^chat\.broadcast' | ls_normalize
ppz_a read chat.broadcast >/dev/null
echo "--- after read ---"
ppz_a ls | grep '^chat\.broadcast' | ls_normalize
