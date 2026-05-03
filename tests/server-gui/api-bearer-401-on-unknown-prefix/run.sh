#!/usr/bin/env bash
# Phase 2: requireBearer rejects tokens with neither the ppz_live_
# (api_key) nor ppz_oauth_ (oauth_tokens) prefix without even hitting
# the DB. Cheap defence against random scanner traffic.
. /tests/lib/common.sh

echo "--- random bearer that doesn't match either prefix ---"
curl_server "/api/v1/sources" -H "Authorization: Bearer something_random_abcdef" \
    --max-redirs 0 -o /dev/null -w "status=%{http_code}\n" -s

echo ""
echo "--- ppz_live_ but wrong hash → still 401 ---"
curl_server "/api/v1/sources" -H "Authorization: Bearer ppz_live_deadbeef" \
    --max-redirs 0 -o /dev/null -w "status=%{http_code}\n" -s

echo ""
echo "--- ppz_oauth_ but unknown → 401 ---"
curl_server "/api/v1/sources" -H "Authorization: Bearer ppz_oauth_definitelynotrealexample" \
    --max-redirs 0 -o /dev/null -w "status=%{http_code}\n" -s
