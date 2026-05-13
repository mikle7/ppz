#!/usr/bin/env bash
# Reusing an existing handle for `ppz terminal create` → E_HANDLE_TAKEN.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create existing >/dev/null
ppz_a terminal create existing
