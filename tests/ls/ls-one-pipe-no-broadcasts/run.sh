#!/usr/bin/env bash
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create only >/dev/null
# No message yet → both ts and payload columns are '-'.
ppz_a ls | ls_normalize
