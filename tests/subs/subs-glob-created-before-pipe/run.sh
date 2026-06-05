#!/usr/bin/env bash
# Subscribe to a glob before any matching pipe exists: subs ls shows the
# pattern as a parent row with a `(no matches)` leaf — visible, not vanished,
# so the user can see "subscribed, catching nothing". Create a matching
# source later → it appears as a child, because the pattern is re-evaluated
# against live sources at read-time rather than frozen at add-time.
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
export PPZ_SESSION=mysh
ppz_a subs add 'room-%.inbox' >/dev/null
echo "--- before any matching pipe ---"
ppz_a subs ls | ls_normalize
ppz_a source create room-a >/dev/null
echo "--- after matching pipe added ---"
ppz_a subs ls | ls_normalize
