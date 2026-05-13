#!/usr/bin/env bash
# A source created via an OAuth bearer (ppz_oauth_… token) inherits the
# bearer's user. Drives the device flow as foo (alpha owner), uses the
# minted bearer to POST /api/v1/sources?org=<alpha>, then verifies the
# server-side row carries foo as creator (via /api/v1/sources GET).
. /tests/lib/common.sh

# 0. Browser-side session for foo.
COOKIE_JAR=$(mktemp)
trap "rm -f $COOKIE_JAR" EXIT
curl_server "/dev/login?user=foo" -X POST -c "$COOKIE_JAR" -o /dev/null -s

# 1. Mint device codes.
mint=$(curl_server "/oauth/device/code" -X POST -s)
DEVICE_CODE=$(echo "$mint" | grep -oE '"device_code":"[^"]+"' | sed -E 's/.*"([^"]+)"/\1/')
USER_CODE=$(echo "$mint"   | grep -oE '"user_code":"[^"]+"'   | sed -E 's/.*"([^"]+)"/\1/')

# 2. Approve as foo.
curl_server "/oauth/device/verify" -X POST -b "$COOKIE_JAR" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    --data-urlencode "user_code=$USER_CODE" \
    --max-redirs 0 -o /dev/null -s

# 3. Exchange device_code for bearer.
token=$(curl_server "/oauth/device/token" -X POST \
    -H "Content-Type: application/json" \
    -d "{\"device_code\":\"$DEVICE_CODE\"}" -s)
BEARER=$(echo "$token" | grep -oE '"access_token":"[^"]+"' | sed -E 's/.*"([^"]+)"/\1/')

# 4. Create the source via the OAuth bearer, scoped to alpha.
ORG_ALPHA=$(cat /seed/org-alpha.txt)
curl_server "/api/v1/sources?org=$ORG_ALPHA" -X POST \
    -H "Authorization: Bearer $BEARER" \
    -H "Content-Type: application/json" \
    -d '{"handle":"oauth-source"}' \
    -o /dev/null -w "create_status=%{http_code}\n" -s

# 5. List sources via the same bearer; project the new source's creator.
curl_server "/api/v1/sources?org=$ORG_ALPHA" \
    -H "Authorization: Bearer $BEARER" -s \
  | jq -c '.sources[] | select(.handle=="oauth-source") | {handle, created_by}'
