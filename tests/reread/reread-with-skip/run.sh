#!/usr/bin/env bash
# --skip N drops the first N retained messages. Lives on `reread`
# (the historical / forensic verb) — `read` is cursor-driven and has
# no filter flags.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
for i in 1 2 3 4 5; do
  ppz_a broadcast -m "msg-$i" >/dev/null
done
wait_for 20 "ppz_a ls | grep -q msg-5" >/dev/null

ppz_a reread chat.broadcast --skip 3
