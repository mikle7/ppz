#!/usr/bin/env bash
# Confirmation (NOT a bug): `ppz reread` ignores the session cursor and
# replays the full retained stream, so it is immune to the stale-cursor
# wedge that breaks `ppz read` after a source is destroyed + recreated.
# This is the workaround agents reached for in production; lock it in so
# the upcoming cursor-invalidation fix doesn't regress reread's forensic
# semantics. Expected GREEN both before and after the fix.
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
wait_for 20 "ppz_a ls | grep -q new-2" >/dev/null

echo "--- reread replays the recreated stream regardless of stale cursor ---"
ppz_a reread --bare alex.inbox
