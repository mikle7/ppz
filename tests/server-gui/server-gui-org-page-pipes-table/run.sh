#!/usr/bin/env bash
# The org page renders sources as a table. Each row exposes a stable
#   data-source-row="<handle>:<pipe>:<ts-or-empty>:<payload-or-empty>"
# marker, and the pipe cell holds a
#   data-source-pipe-link="/accounts/<slug>/sources/<handle>/pipes/<pipe>"
# anchor pointing at the pipe detail page.
. /tests/lib/common.sh
auth_as_foo

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create chat >/dev/null
ppz_a send chat.inbox "hello table" >/dev/null
wait_for 20 "ppz_a ls | grep -q 'hello table'" >/dev/null

PAGE="$(curl_server "/accounts/alpha")"

echo "$PAGE" \
  | grep -oE 'data-source-row="[^"]+"' \
  | sed -E 's/data-source-row="([^"]+)"/\1/' \
  | sed -E 's/:(just now|[0-9]+ (seconds?|minutes?|hours?|days?) ago):/:RELATIVE:/'

echo "$PAGE" \
  | grep -oE 'data-source-pipe-link="[^"]+"' \
  | sed -E 's/data-source-pipe-link="([^"]+)"/\1/'
