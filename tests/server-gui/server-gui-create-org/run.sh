#!/usr/bin/env bash
. /tests/lib/common.sh
auth_as_bar
# POST a new org, then verify it appears on the index alongside the seeded orgs.
curl_server /accounts -X POST --data-urlencode "name=gamma" -o /dev/null
curl_server /dashboard | grep -oE 'data-account="[^"]+"' | sed -E 's/data-account="([^"]+)"/\1/' | sort
