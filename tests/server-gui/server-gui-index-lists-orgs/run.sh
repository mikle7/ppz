#!/usr/bin/env bash
. /tests/lib/common.sh
auth_as_bar
# / lists every org. Extract data-account markers, sort alphabetically.
curl_server /dashboard | grep -oE 'data-account="[^"]+"' | sed -E 's/data-account="([^"]+)"/\1/' | sort
