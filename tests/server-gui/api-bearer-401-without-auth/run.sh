#!/usr/bin/env bash
# Phase 2: every authenticated /api/* route must reject anonymous
# callers with 401. Smoke a representative endpoint.
. /tests/lib/common.sh

echo "--- GET /api/v1/sources without Authorization ---"
curl_server "/api/v1/sources" --max-redirs 0 -o /dev/null \
    -w "status=%{http_code}\n" -s

echo ""
echo "--- with garbage scheme ---"
curl_server "/api/v1/sources" -H "Authorization: Basic dXNlcjpwYXNz" \
    --max-redirs 0 -o /dev/null -w "status=%{http_code}\n" -s

echo ""
echo "--- with empty Bearer ---"
curl_server "/api/v1/sources" -H "Authorization: Bearer " \
    --max-redirs 0 -o /dev/null -w "status=%{http_code}\n" -s
