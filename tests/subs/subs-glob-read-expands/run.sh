#!/usr/bin/env bash
# `subs read` over a glob sub expands to each matching pipe with unread,
# each block prefixed by its own `=== <target> ===` separator.
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
export PPZ_SESSION=mysh
ppz_a source create room-a >/dev/null
ppz_a source create room-b >/dev/null
ppz_a subs add 'room-%.inbox' >/dev/null
ppz_a send room-a.inbox from-a >/dev/null
ppz_a send room-b.inbox from-b >/dev/null
wait_for 20 "ppz_a ls | grep -q from-b" >/dev/null
OUT=$(mktemp)
ppz_a subs read >"$OUT" 2>/dev/null
grep '^=== ' "$OUT"
echo "--- payloads ---"
grep -oE 'from-[ab]' "$OUT"
