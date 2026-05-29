#!/usr/bin/env bash
# Corner case of read-survives-source-recreate: when MORE messages are
# sent to the recreated source than the stale cursor's value, the bug
# becomes SILENT DATA LOSS rather than a visible wedge.
#
# Old cursor = 3 (three messages read pre-recreate). After recreate the
# new stream restarts at seq 1; sending 5 messages puts LastSeq at 5.
# Both buggy paths now partially work against the wrong baseline:
#   - ls computes unread = LastSeq(5) - cursor(3) = 2  (undercount)
#   - read starts at cursor+1 = 4, draining only seq 4..5 and SILENTLY
#     DROPPING new-1..new-3 — messages from a brand-new stream the
#     session has never actually read.
#
# A `cursor > LastSeq` clamp does NOT catch this (3 < 5). The correct
# behaviour: a recreated source is a fresh stream the session has read
# nothing from, so all 5 messages are unread and drained.
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
for i in 1 2 3 4 5; do ppz_a send alex.inbox "new-$i" >/dev/null; done
wait_for 20 "ppz_a ls | grep -q new-5" >/dev/null

echo "--- after recreate: ls counts all 5 as unread ---"
ppz_a ls | ls_normalize | awk '$1 == "alex.inbox" {print "unread="$2, "buffered="$3}'

echo "--- after recreate: read drains all 5 (no silent drop) ---"
ppz_a read --bare alex.inbox
