#!/usr/bin/env bash
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create foo >/dev/null
ppz_a broadcast -m "hi from arg"
wait_for 20 "ppz_a ls | grep -q 'hi from arg'" >/dev/null
ppz_a ls | ls_normalize
