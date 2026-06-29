#!/usr/bin/env bash
# The verify-page org dropdown defaults to the org the user LAST authorized
# into, not just their default org. bar's default is alpha; after they
# authorize a session into beta, the next login's dropdown defaults to beta.
. /tests/lib/common.sh

COOKIE_JAR=$(mktemp)
trap "rm -f $COOKIE_JAR" EXIT
curl_server "/dev/login?user=bar" -X POST -c "$COOKIE_JAR" -o /dev/null -s

# First login: approve into beta (NOT bar's default org).
mint1=$(curl_server "/oauth/device/code" -X POST -H "Content-Type: application/json" -d '{}' -s)
UC1=$(echo "$mint1" | grep -oE '"user_code":"[^"]+"' | sed -E 's/.*:"([^"]+)"/\1/')
page1=$(curl_server "/oauth/device/verify?user_code=$UC1" -b "$COOKIE_JAR" -s)
BETA_ID=$(echo "$page1" | grep -oE '<option value="[0-9a-f-]+"[^>]*>beta</option>' | grep -oE '[0-9a-f-]{36}' | head -1)
echo "beta_id_found: $([[ -n "$BETA_ID" ]] && echo true || echo false)"
curl_server "/oauth/device/verify" -X POST -b "$COOKIE_JAR" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    --data-urlencode "user_code=$UC1" \
    --data-urlencode "account_id=$BETA_ID" \
    --max-redirs 0 -o /dev/null -s

# Second login: the dropdown should now default to beta.
mint2=$(curl_server "/oauth/device/code" -X POST -H "Content-Type: application/json" -d '{}' -s)
UC2=$(echo "$mint2" | grep -oE '"user_code":"[^"]+"' | sed -E 's/.*:"([^"]+)"/\1/')
page2=$(curl_server "/oauth/device/verify?user_code=$UC2" -b "$COOKIE_JAR" -s)
echo "default_selected: $(echo "$page2" | grep -oE '<option value="[^"]*" selected>[^<]+</option>' | sed -E 's/.*>([^<]+)<.*/\1/')"
