#!/usr/bin/env bash
# Reusing an existing handle for `ppz source create` → E_SOURCE_TAKEN.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create existing >/dev/null
ppz_a source create existing
