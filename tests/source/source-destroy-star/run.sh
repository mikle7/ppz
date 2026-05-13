#!/usr/bin/env bash
# `ppz source destroy '*'` destroys every source in the org.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create alpha >/dev/null
ppz_a source create beta >/dev/null

echo "--- before destroy ---"
ppz_a ls | ls_normalize

ppz_a source destroy '*' | sort

echo "--- after destroy ---"
ppz_a ls | { grep '' || echo "empty"; }
