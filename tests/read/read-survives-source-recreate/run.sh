#!/usr/bin/env bash
# Bug repro: destroying a source and recreating it under the same
# handle resets its JetStream stream (seq restarts at 1), but the
# per-session read cursor persists at its old, higher value. After
# recreate, `ppz ls` reports unread=0 and `ppz read` returns nothing
# even though a fresh message is buffered — the stale cursor sits
# ahead of the new stream's LastSeq, so:
#   - list_snapshot.go gates unread on LastSeq > cursor (1 > 3 false) → 0
#   - read.go sets startSeq = cursor+1 (4), past LastSeq (1) → no drain,
#     and lastSeqSeen stays 0 so the cursor never advances. The wedge
#     is permanent: every future `read` is a silent no-op.
#
# Fix: a cursor ahead of the stream's LastSeq is impossible for a
# healthy monotonic stream; treat it as stale and clamp to the start
# of the stream before computing unread / the read start sequence.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create alex >/dev/null
ppz_a send alex.inbox "msg-1" >/dev/null
ppz_a send alex.inbox "msg-2" >/dev/null
ppz_a send alex.inbox "msg-3" >/dev/null
wait_for 20 "ppz_a ls | grep -q msg-3" >/dev/null

# Drain all three so the session cursor advances to seq 3.
echo "--- before recreate: read drains all 3 ---"
ppz_a read --bare alex.inbox

# Destroy + recreate under the same handle: same account, same cursor
# key (<account>.alex.inbox), but a brand-new stream whose seq restarts
# at 1.
ppz_a source destroy alex >/dev/null
ppz_a source create alex >/dev/null
ppz_a send alex.inbox "after-recreate" >/dev/null
wait_for 20 "ppz_a ls | grep -q after-recreate" >/dev/null

echo "--- after recreate: ls counts the new message as unread ---"
ppz_a ls | ls_normalize | awk '$1 == "alex.inbox" {print "unread="$2, "buffered="$3}'

echo "--- after recreate: read drains the new message ---"
ppz_a read --bare alex.inbox
