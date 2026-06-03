#!/usr/bin/env bash
# A fully-specified literal that matches no existing pipe warns the user
# (steering to the glob form) and returns no rows — like `ls Mus` → "No
# such file". A glob is the speculative form.
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
out=$(mktemp); err=$(mktemp)
ppz_a ls ghostpipe >"$out" 2>"$err"
echo "stdout-empty=$([ -s "$out" ] && echo no || echo yes)"
grep -qi 'no pipe matches' "$err" && echo "warned=yes" || echo "warned=no"
