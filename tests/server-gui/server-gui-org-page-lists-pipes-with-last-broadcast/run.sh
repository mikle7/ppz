#!/usr/bin/env bash
. /tests/lib/common.sh
auth_as_foo
# Arrange: create a pipe and broadcast something via daemon-a (org alpha),
# then scrape the GUI org page for the table row marker.
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a broadcast -m "hello gui" >/dev/null
wait_for 20 "ppz_a ls | grep -q 'hello gui'" >/dev/null

org_id="$(cat /seed/org-alpha.txt)"
curl_server "/orgs/$org_id" \
  | grep -oE 'data-source-row="[^"]+"' \
  | sed -E 's/data-source-row="([^"]+)"/\1/' \
  | sed -E 's/:(just now|[0-9]+ (seconds?|minutes?|hours?|days?) ago):/:RELATIVE:/'
