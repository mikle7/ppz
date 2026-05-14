#!/usr/bin/env bash
# Phase 1.5.1 sender identity: uncollared sends stamp envelope.sender
# = "" (empty). The user explicitly chose this over current_handle to
# keep uncollared semantically "no actor" — future iteration may
# default to the account username, but for now empty is the contract.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create alice >/dev/null
# alice is now current handle. Create uncollared `room`. Send to it.
# The send should stamp sender="" (uncollared), NOT "alice".
ppz_a unset handle >/dev/null
ppz_a pipe create room >/dev/null
ppz_a set handle alice >/dev/null

err=$(mktemp)
ppz_a send room "hi from uncollared" 2>"$err"
grep -oE '^sent id=[a-f0-9]{8} to=[^ ]+ bytes=[0-9]+$' "$err" | head -1 \
  | sed -E 's/id=[a-f0-9]{8}/id=ID8/; s/bytes=[0-9]+/bytes=N/'

ppz_a reread room -l 1 --json | head -1 \
  | sed -E 's/.*"sender":"([^"]*)".*/sender=\1/'
rm -f "$err"
