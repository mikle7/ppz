#!/usr/bin/env bash
# Boundary corner case of read-survives-source-recreate: when the
# recreated source receives EXACTLY as many messages as the stale
# cursor's value, a naive `cursor > LastSeq` guard wouldn't fire
# (cursor == LastSeq), leaving the wedge fully in place.
#
# Old cursor = 3. After recreate, send exactly 3 → LastSeq = 3 = cursor.
#   - ls: LastSeq(3) > cursor(3) is false        -> unread = 0
#   - read: startSeq = cursor+1 = 4 > LastSeq(3) -> no drain, nothing
#
# Correct: fresh stream, 3 unread, read drains all 3. This is why the
# fix must key on stream identity (recreation), not on a LastSeq
# comparison.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create alex >/dev/null
ppz_a send alex.inbox "old-1" >/dev/null
ppz_a send alex.inbox "old-2" >/dev/null
ppz_a send alex.inbox "old-3" >/dev/null
wait_for 20 "ppz_a ls | grep -q old-3" >/dev/null
ppz_a read alex.inbox >/dev/null   # cursor -> 3

ppz_a source destroy alex >/dev/null
ppz_a source create alex >/dev/null
ppz_a send alex.inbox "new-1" >/dev/null
ppz_a send alex.inbox "new-2" >/dev/null
ppz_a send alex.inbox "new-3" >/dev/null
wait_for 20 "ppz_a ls | grep -q new-3" >/dev/null

echo "--- after recreate: ls counts all 3 as unread ---"
ppz_a ls | ls_normalize | awk '$1 == "alex.inbox" {print "unread="$2, "buffered="$3}'

echo "--- after recreate: read drains all 3 ---"
ppz_a read --bare alex.inbox
