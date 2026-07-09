#!/usr/bin/env bash
# --limit is the long form of -l on `ppz subs read`: same per-pipe
# head-N cap, same -l 0 opt-out. Both spellings must behave identically.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
export PPZ_SESSION=mysh
ppz_a source create foo >/dev/null
ppz_a subs add foo.inbox >/dev/null
ppz_a send foo.inbox "foo-1" >/dev/null
ppz_a send foo.inbox "foo-2" >/dev/null
ppz_a send foo.inbox "foo-3" >/dev/null
wait_for 20 "ppz_a subs ls | grep -q foo-3" >/dev/null

OUT=$(mktemp)
ppz_a subs read --limit 1 >"$OUT" 2>/dev/null
echo "--- subs read --limit 1: one message + trailer ---"
grep '^=== ' "$OUT"
grep -oE 'foo-[0-9]+' "$OUT"
grep -oE '[0-9]+ more unread' "$OUT"

ppz_a subs read --limit 0 >"$OUT" 2>/dev/null
echo "--- subs read --limit 0: drains the rest ---"
grep -oE 'foo-[0-9]+' "$OUT"
