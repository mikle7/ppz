#!/usr/bin/env bash
# Org page must surface "last message" + payload for *every* pipe, not
# just one. Column header is "Last Message" (not "Last Broadcast").
# Time renders as a relative duration ("X seconds ago" / "just now") so
# operators see freshness at a glance without parsing timestamps.
. /tests/lib/common.sh
auth_as_foo

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create foo >/dev/null
ppz_a pipe create archive >/dev/null
ppz_a send foo.inbox "hello-inbox" >/dev/null
ppz_a send foo.archive "hello-archive" >/dev/null
wait_for 20 "ppz_a ls | grep -q hello-archive" >/dev/null

PAGE=$(curl_server "/orgs/alpha")

echo "--- column header is 'Last Message' ---"
echo "$PAGE" | grep -oE '<th>[^<]*</th>' | sed -E 's/.*<th>(.+)<\/th>.*/\1/' | grep -iE '^(last broadcast|last message)$' | head -1

echo "--- per-pipe rows: handle:pipe:RELATIVE:payload (inbox + archive both populated) ---"
echo "$PAGE" \
  | grep -oE 'data-source-row="[^"]+"' \
  | sed -E 's/data-source-row="([^"]+)"/\1/' \
  | sed -E 's/:(just now|[0-9]+ (seconds?|minutes?|hours?|days?) ago):/:RELATIVE:/' \
  | sort
