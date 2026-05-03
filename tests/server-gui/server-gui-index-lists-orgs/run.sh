#!/usr/bin/env bash
. /tests/lib/common.sh
auth_as_bar
# / lists every org. Extract data-org markers, sort alphabetically.
curl_server /dashboard | grep -oE 'data-org="[^"]+"' | sed -E 's/data-org="([^"]+)"/\1/' | sort
