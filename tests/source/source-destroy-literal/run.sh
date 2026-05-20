#!/usr/bin/env bash
# `ppz source destroy HANDLE` removes a specific source and all its pipes.
# After destroy, ls no longer shows the source.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create apple >/dev/null

echo "--- before destroy ---"
ppz_a ls | ls_normalize | grep '^apple\.'

ppz_a source destroy apple

echo "--- after destroy ---"
ppz_a ls | ls_normalize | grep '^apple\.' || echo "no apple rows"
