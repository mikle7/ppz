#!/usr/bin/env bash
# Reading a handle that doesn't exist in the org → E_PIPE_NOT_FOUND.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a read ghost.broadcast
