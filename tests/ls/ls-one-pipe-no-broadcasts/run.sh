#!/usr/bin/env bash
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create only >/dev/null
# No broadcast yet → both ts and payload columns are '-'.
ppz_a ls | ls_normalize
