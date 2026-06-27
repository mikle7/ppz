#!/usr/bin/env bash
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create alpha >/dev/null
ppz_a source create beta  >/dev/null
# After two creates, 'current' is the most-recently-created handle.
ppz_a status | grep '^current source:'
