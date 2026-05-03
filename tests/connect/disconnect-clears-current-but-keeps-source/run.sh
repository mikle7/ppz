#!/usr/bin/env bash
# `ppz source clear` clears the daemon's current source but leaves the source
# itself provisioned (still discoverable via ls).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a source clear

echo "--- status ---"
ppz_a status | grep '^current source:'
echo "--- ls ---"
ppz_a ls | grep '^chat\.broadcast' | ls_normalize
