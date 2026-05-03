#!/usr/bin/env bash
# Owner-only gate (positive case): the org owner CAN revoke a key.
# Mirrors auth-revoke-key-non-owner-403 but using foo, who owns alpha.
. /tests/lib/common.sh

COOKIE_JAR=$(mktemp)
trap "rm -f $COOKIE_JAR" EXIT

curl_server "/dev/login?user=foo" -X POST -c "$COOKIE_JAR" -o /dev/null -s

ORG_ID=$(cat /seed/org-alpha.txt)
KEY_ID=$(curl_server "/orgs/$ORG_ID/keys" -b "$COOKIE_JAR" -s \
           | grep -oE 'data-key-id="[^"]+"' | head -1 \
           | sed -E 's/data-key-id="([^"]+)"/\1/')

echo "--- POST revoke as foo (owner of alpha) ---"
curl_server "/orgs/$ORG_ID/keys/$KEY_ID/revoke" -X POST -b "$COOKIE_JAR" \
    --max-redirs 0 -o /dev/null -w "status=%{http_code}\n" -s

echo "--- key state after revoke ---"
curl_server "/orgs/$ORG_ID/keys" -b "$COOKIE_JAR" -s \
  | grep -A 1 "data-key-id=\"$KEY_ID\"" \
  | grep -oE 'data-key-state="[^"]+"' | head -1
