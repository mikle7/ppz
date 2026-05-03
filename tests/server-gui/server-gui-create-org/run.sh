#!/usr/bin/env bash
. /tests/lib/common.sh
auth_as_bar
# POST a new org, then verify it appears on the index alongside the seeded orgs.
curl_server /orgs -X POST --data-urlencode "name=gamma" -o /dev/null
curl_server /dashboard | grep -oE 'data-org="[^"]+"' | sed -E 's/data-org="([^"]+)"/\1/' | sort
