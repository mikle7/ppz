#!/usr/bin/env bash
# Creating a pipe with a name that's already taken on the same source
# returns E_PIPE_TAKEN (exit 21). The first creation succeeds; the second
# fails with the standard error envelope.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create chat >/dev/null
ppz_a pipe create archive >/dev/null
ppz_a pipe create archive
