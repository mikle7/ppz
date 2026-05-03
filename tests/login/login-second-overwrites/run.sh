#!/usr/bin/env bash
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)"
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha2)"
ppz_a status
