#!/usr/bin/env bash
# `ppz send <handle> <payload>` targets <handle>.inbox.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a send chat "hello inbox" >/dev/null

wait_for 20 "ppz_a ls | grep -q 'hello inbox'" >/dev/null
ppz_a read --bare inbox
