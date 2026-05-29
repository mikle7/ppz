#!/usr/bin/env bash
# Cross-daemon proof for the stale-cursor fix, and the reason it must be
# reactive rather than a cleanup hook on destroy/create.
#
# The cursor that goes stale lives on the daemon that READ the source,
# but destroy+recreate happens on a DIFFERENT daemon. Cursors are
# daemon-local, per-session files (<PPZ_HOME>/cursors/<session>.json) and
# destroy is a server-side op (DELETE /api/v1/sources/{handle}) — so a
# "drop the cursor when the source is destroyed" fix could only ever
# touch the destroyer's own daemon, never the reader's. Only a reactive
# check (compare the live stream's identity at read time) un-wedges the
# reader. This mirrors the production incident: the operator destroyed
# the agent from one box while the agent's stale cursor sat on another.
#
# A creates alex and sends 3; B reads them (B's cursor -> 3, stamped
# against the original stream). A then destroys + recreates alex and
# sends one fresh message. B must see it on its next read — its stale
# cursor (3) sits ahead of the recreated stream's LastSeq (1).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)"  >/dev/null
ppz_b daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha2)" >/dev/null

ppz_a source create alex >/dev/null
ppz_a send alex.inbox "msg-1" >/dev/null
ppz_a send alex.inbox "msg-2" >/dev/null
ppz_a send alex.inbox "msg-3" >/dev/null

# B reads from its own daemon: advances B's local cursor to seq 3.
wait_for 20 "ppz_b ls | grep -q msg-3" >/dev/null
echo "--- B drains all 3 (cursor -> 3 on daemon B) ---"
ppz_b read --bare alex.inbox

# Destroy + recreate happens entirely on daemon A. Daemon B never
# observes it and its cursor file is untouched.
ppz_a source destroy alex >/dev/null
ppz_a source create alex >/dev/null
ppz_a send alex.inbox "after-recreate" >/dev/null
wait_for 20 "ppz_b ls | grep -q after-recreate" >/dev/null

echo "--- B counts the recreated source's message as unread ---"
ppz_b ls | ls_normalize | awk '$1 == "alex.inbox" {print "unread="$2, "buffered="$3}'

echo "--- B drains the new message despite its stale cursor ---"
ppz_b read --bare alex.inbox
