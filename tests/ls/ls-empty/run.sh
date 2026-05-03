#!/usr/bin/env bash
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
# No pipes created → ls produces zero stdout, exit 0.
ppz_a ls
