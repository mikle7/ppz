#!/usr/bin/env bash
# Owner-only gate: a member who is not the org owner cannot revoke a
# key. Returns 403, the key remains usable.
#
# Setup (provided by the seed): foo owns alpha, bar is a non-owner
# member of alpha.
. /tests/lib/common.sh

COOKIE_JAR=$(mktemp)
trap "rm -f $COOKIE_JAR" EXIT

# Authenticate as bar (member, not owner) via the dev login endpoint.
curl_server "/dev/login?user=bar" -X POST -c "$COOKIE_JAR" -o /dev/null -s

ORG_ID=$(cat /seed/org-alpha.txt)
KEY_ID=$(curl_server "/orgs/$ORG_ID/keys" -b "$COOKIE_JAR" -s \
           | grep -oE 'data-key-id="[^"]+"' | head -1 \
           | sed -E 's/data-key-id="([^"]+)"/\1/')

echo "--- POST revoke as bar (non-owner of alpha) ---"
curl_server "/orgs/$ORG_ID/keys/$KEY_ID/revoke" -X POST -b "$COOKIE_JAR" \
    --max-redirs 0 -o /dev/null -w "status=%{http_code}\n" -s

echo "--- key state after the rejected revoke (should still be active) ---"
# data-key-id and data-key-state are on adjacent lines in org.html;
# `grep -A 1` pulls the state line that follows the id we care about.
curl_server "/orgs/$ORG_ID/keys" -b "$COOKIE_JAR" -s \
  | grep -A 1 "data-key-id=\"$KEY_ID\"" \
  | grep -oE 'data-key-state="[^"]+"' | head -1
