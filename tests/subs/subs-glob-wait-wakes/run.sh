#!/usr/bin/env bash
# `subs wait` on a glob sub wakes when any matching pipe gets unread, and
# prints only that unread row.
. /tests/lib/common.sh
cleanup() { kill "$WPID" 2>/dev/null || true; wait "$WPID" 2>/dev/null || true; }
trap cleanup EXIT
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
export PPZ_SESSION=mysh
ppz_a source create room-a >/dev/null
ppz_a source create room-b >/dev/null
ppz_a subs add 'room-%.inbox' >/dev/null
OUT=$(mktemp)
ppz_a subs wait >"$OUT" 2>/dev/null &
WPID=$!
sleep 0.6
ppz_a send room-b.inbox hey >/dev/null
wait "$WPID" 2>/dev/null; WPID=
ls_normalize <"$OUT" | awk '{print $1, $2}'
