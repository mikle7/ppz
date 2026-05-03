#!/usr/bin/env bash
# Happy path: read all retained messages from a pipe's broadcast channel,
# default text-payload output, oldest first.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a broadcast -m "msg-1" >/dev/null
ppz_a broadcast -m "msg-2" >/dev/null
ppz_a broadcast -m "msg-3" >/dev/null
wait_for 20 "ppz_a ls | grep -q msg-3" >/dev/null

ppz_a read chat.broadcast
