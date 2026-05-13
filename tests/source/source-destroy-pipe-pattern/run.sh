#!/usr/bin/env bash
# `ppz source destroy '*.PIPE'` destroys a named pipe across all sources
# that have it, leaving the sources and other pipes intact.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create apple >/dev/null
ppz_a source create banana >/dev/null
ppz_a pipe create apple.custom >/dev/null
ppz_a pipe create banana.custom >/dev/null

echo "--- before destroy ---"
ppz_a ls | ls_normalize

ppz_a source destroy '*.custom' | sort

echo "--- after destroy ---"
ppz_a ls | ls_normalize
