#!/usr/bin/env bash
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
# No 'create' or 'switch' → no current handle. broadcast must exit 16.
ppz_a broadcast -m "no target"
