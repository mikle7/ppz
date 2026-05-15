#!/usr/bin/env bash
# Phase 1.5.3 sender identity (revised from 1.5.1): uncollared sends
# stamp envelope.sender = current_handle when a current handle is
# set. The pre-1.5.3 behaviour stamped empty regardless — that lost
# identity in mixed-agent inboxes ("who sent this?" became
# unanswerable in tabular `ppz read` output). Falls back to empty
# when no current handle is set; that case is covered by the
# companion test send-uncollared-stamps-empty-without-handle.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset namespace >/dev/null
ppz_a source create alice >/dev/null
# alice is now current handle. Create uncollared `room`. Send to it.
# The send should stamp sender="alice" (matching the session's
# current_handle), NOT "".
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
