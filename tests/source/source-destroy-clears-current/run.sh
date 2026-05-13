#!/usr/bin/env bash
# Destroying a source that is the session's current clears the current
# binding. Subsequent status shows no current source.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create apple >/dev/null
ppz_a source switch apple >/dev/null

echo "--- current before destroy ---"
ppz_a status | grep '^current source:'

ppz_a source destroy apple >/dev/null

echo "--- current after destroy ---"
ppz_a status | grep '^current source:'
