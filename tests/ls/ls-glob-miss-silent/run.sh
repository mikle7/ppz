#!/usr/bin/env bash
# A glob that currently matches nothing is the speculative form — no
# warning (you may be watching/listing for something not created yet).
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
err=$(mktemp)
ppz_a ls 'ghost-%' >/dev/null 2>"$err"
grep -qi 'no pipe matches' "$err" && echo "warned=yes" || echo "warned=no"
