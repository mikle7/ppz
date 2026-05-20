#!/usr/bin/env bash
# `ppz unset handle` clears the daemon's current source but leaves the source
# itself provisioned (still discoverable via ls).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a unset handle

echo "--- status ---"
ppz_a status | grep '^current source:'
echo "--- ls ---"
ppz_a ls | ls_normalize | grep '^chat\.inbox'
