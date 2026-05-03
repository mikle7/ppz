#!/usr/bin/env bash
# /orgs/<slug>/sources/<handle>/pipes/<pipe> lists every buffered message
# from the JetStream stream backing the (source, pipe), in chronological
# order. Each <tr> exposes a stable
#   data-message="<id>:<created_at>:<payload>"
# marker so tests don't depend on the surrounding layout.
. /tests/lib/common.sh
auth_as_foo

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a broadcast -m "msg-1" >/dev/null
ppz_a broadcast -m "msg-2" >/dev/null
ppz_a broadcast -m "msg-3" >/dev/null
wait_for 20 "ppz_a ls | grep -q msg-3" >/dev/null

curl_server "/orgs/alpha/sources/chat/pipes/broadcast" \
  | grep -oE 'data-message="[^"]+"' \
  | sed -E 's/data-message="([^"]+)"/\1/'
