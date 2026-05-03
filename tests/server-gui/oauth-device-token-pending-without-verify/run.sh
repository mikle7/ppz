#!/usr/bin/env bash
# Phase 2: polling /oauth/device/token before the user has verified
# the user_code in the browser returns RFC 8628 "authorization_pending"
# with HTTP 400 (per spec).
. /tests/lib/common.sh

# Mint a fresh device code (no user verification yet)
DEVICE_CODE=$(curl_server "/oauth/device/code" -X POST -s | \
    grep -oE '"device_code":"[^"]+"' | sed -E 's/"device_code":"([^"]+)"/\1/')
echo "device_code_minted: $([[ -n "$DEVICE_CODE" ]] && echo true || echo false)"

echo ""
echo "--- poll without verification → authorization_pending ---"
body=$(curl_server "/oauth/device/token" -X POST \
    -H "Content-Type: application/json" \
    -d "{\"device_code\":\"$DEVICE_CODE\"}" \
    --max-redirs 0 -s -w "status=%{http_code}\n")
echo "$body" | grep -oE '"error":"[^"]+"|status=[0-9]+'
