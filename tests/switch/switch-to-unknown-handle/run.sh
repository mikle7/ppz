#!/usr/bin/env bash
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
# 'ghost' was never created → E_PIPE_NOT_FOUND, exit 14, no stdout.
ppz_a set handle ghost
