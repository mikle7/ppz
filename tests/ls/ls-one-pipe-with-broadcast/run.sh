#!/usr/bin/env bash
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create chat >/dev/null
ppz_a send chat.inbox "first message" >/dev/null
wait_for 20 "ppz_a ls | grep -q 'first message'" >/dev/null
ppz_a ls | ls_normalize
