#!/usr/bin/env bash
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create one >/dev/null
ppz_a terminal create two >/dev/null
# 'two' is current after create. Switch back to 'one' and verify.
ppz_a set handle one
ppz_a status | grep '^current source:'
