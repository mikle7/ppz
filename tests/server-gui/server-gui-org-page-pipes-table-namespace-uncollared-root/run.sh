#!/usr/bin/env bash
# An uncollared pipe at root must render NAMESPACE="-" in the org
# pipes table. Uncollared rows have empty handle slot in
# data-source-row, so the marker is `:<pipe>::`.
. /tests/lib/common.sh
auth_as_foo

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle    >/dev/null 2>&1
ppz_a unset namespace >/dev/null 2>&1
ppz_a pipe create plaza >/dev/null
wait_for 20 "ppz_a ls | grep -q plaza" >/dev/null

PAGE="$(curl_server "/orgs/alpha")"

echo "$PAGE" \
  | tr '>' '\n' \
  | grep -E 'data-source-row=":plaza:' \
  | grep -oE 'data-source-namespace="[^"]*"' \
  | sed -E 's/data-source-namespace="([^"]*)"/namespace=\1/'
