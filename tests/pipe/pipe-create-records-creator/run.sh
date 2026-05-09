#!/usr/bin/env bash
# `ppz pipe create` records the creating user against the new pipe row.
# alpha-primary→foo creates a source + custom pipe. `ppz ls --json`
# carries `human=foo` on every row, including the user-created pipe.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a pipe create archive >/dev/null

ppz_a ls --json | jq -c '{handle, pipe, human}'
