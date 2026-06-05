#!/usr/bin/env bash
# A pattern that currently matches NO pipe must still be visible in
# `subs ls`. Today it vanishes entirely (subsSnapshot skips isGlobPattern
# rows, and there's nothing to expand), so a user can't tell the difference
# between "I never subscribed" and "I'm subscribed but it's catching
# nothing". The parent row renders with a `(no matches)` leaf.
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
export PPZ_SESSION=mysh
ppz_a subs add 'ghost-%' >/dev/null
echo "--- subs ls: zero-match pattern still shown ---"
ppz_a subs ls | ls_normalize
