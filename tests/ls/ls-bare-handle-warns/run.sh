#!/usr/bin/env bash
# `ppz ls alice` does NOT list alice's pipes — there's no uncollared pipe
# named `alice`. It warns, steering to `ppz ls alice%`. (Handle-watch is
# gone; the handle is not a pipe.)
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create alice >/dev/null
out=$(mktemp); err=$(mktemp)
ppz_a ls alice >"$out" 2>"$err"
echo "rows=[$(ls_normalize <"$out" | awk '{print $1}' | tr '\n' ',')]"
grep -qi 'no pipe matches' "$err" && echo "warned=yes" || echo "warned=no"
