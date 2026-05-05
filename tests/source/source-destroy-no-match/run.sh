#!/usr/bin/env bash
# `ppz source destroy HANDLE` with no matching sources prints a count
# and exits 0 — matches `rm -f` semantics.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create apple >/dev/null

ppz_a source destroy ghost

echo "--- apple still exists ---"
ppz_a ls | grep '^apple\.' | ls_normalize
