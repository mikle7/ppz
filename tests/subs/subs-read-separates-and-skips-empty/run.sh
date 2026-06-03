#!/usr/bin/env bash
# subs read = `ppz read` over every subscribed pipe that has unread, each
# block prefixed by a `=== <target> ===` separator so the consumer knows
# which table is which. Subscribed-but-no-unread (baz.inbox) is skipped.
# Targets are visited in sorted order for determinism.
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
export PPZ_SESSION=mysh
ppz_a source create foo >/dev/null
ppz_a source create bar >/dev/null
ppz_a source create baz >/dev/null
ppz_a subs add foo.inbox bar.inbox baz.inbox >/dev/null
ppz_a send foo.inbox "from-foo" >/dev/null
ppz_a send bar.inbox "from-bar" >/dev/null
wait_for 20 "ppz_a subs ls | grep -q from-bar" >/dev/null
OUT=$(mktemp)
ppz_a subs read >"$OUT" 2>/dev/null
echo "--- separators ---"
grep '^=== ' "$OUT"
echo "--- payloads ---"
grep -oE 'from-(foo|bar|baz)' "$OUT"
