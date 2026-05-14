#!/usr/bin/env bash
# /accounts/<slug>/sources/<handle>/pipes/<pipe> lists every buffered message
# from the JetStream stream backing the (source, pipe), in chronological
# order. Each <tr> exposes a stable
#   data-message="<id>:<created_at>:<payload>"
# marker so tests don't depend on the surrounding layout.
. /tests/lib/common.sh
auth_as_foo

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a send chat.inbox "msg-1" >/dev/null
ppz_a send chat.inbox "msg-2" >/dev/null
ppz_a send chat.inbox "msg-3" >/dev/null
wait_for 20 "ppz_a ls | grep -q msg-3" >/dev/null

curl_server "/accounts/alpha/sources/chat/pipes/inbox" \
  | grep -oE 'data-message="[^"]+"' \
  | sed -E 's/data-message="([^"]+)"/\1/'
