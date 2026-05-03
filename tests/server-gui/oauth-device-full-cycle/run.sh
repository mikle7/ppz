#!/usr/bin/env bash
# Phase 2: full device-flow round-trip.
#
#   1. CLI mints device + user codes
#   2. User (signed-in via /dev/login) approves the user_code
#   3. CLI polls /oauth/device/token, gets the bearer
#   4. CLI uses the bearer against /api/v1/sources → 200
. /tests/lib/common.sh

# Step 0: a session for foo (the "user" who'll approve in the browser).
COOKIE_JAR=$(mktemp)
trap "rm -f $COOKIE_JAR" EXIT
curl_server "/dev/login?user=foo" -X POST -c "$COOKIE_JAR" -o /dev/null -s

echo "--- 1. CLI mints device + user codes ---"
mint_body=$(curl_server "/oauth/device/code" -X POST -s)
DEVICE_CODE=$(echo "$mint_body" | grep -oE '"device_code":"[^"]+"' | sed -E 's/"device_code":"([^"]+)"/\1/')
USER_CODE=$(echo "$mint_body" | grep -oE '"user_code":"[^"]+"' | sed -E 's/"user_code":"([^"]+)"/\1/')
echo "minted: $([[ -n "$DEVICE_CODE" && -n "$USER_CODE" ]] && echo true || echo false)"

echo ""
echo "--- 2. user (foo) approves the user_code in the browser ---"
curl_server "/oauth/device/verify" -X POST \
    -b "$COOKIE_JAR" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    --data-urlencode "user_code=$USER_CODE" \
    --max-redirs 0 -o /dev/null -w "verify_status=%{http_code}\n" -s

echo ""
echo "--- 3. CLI polls token endpoint → bearer issued ---"
token_body=$(curl_server "/oauth/device/token" -X POST \
    -H "Content-Type: application/json" \
    -d "{\"device_code\":\"$DEVICE_CODE\"}" \
    -s -w "\nstatus=%{http_code}\n")
TOKEN=$(echo "$token_body" | grep -oE '"access_token":"[^"]+"' | sed -E 's/"access_token":"([^"]+)"/\1/')
echo "token_status: $(echo "$token_body" | grep -oE 'status=[0-9]+')"
echo "token_has_oauth_prefix: $([[ "$TOKEN" == ppz_oauth_* ]] && echo true || echo false)"

echo ""
echo "--- 4. use the bearer against an authed API endpoint ---"
curl_server "/api/v1/sources" \
    -H "Authorization: Bearer $TOKEN" \
    --max-redirs 0 -o /dev/null -w "api_status=%{http_code}\n" -s
