#!/usr/bin/env bash
# /auth/logout clears the session cookie + 303 redirects to /. After
# logout, /dashboard goes back to redirecting to /login.
. /tests/lib/common.sh

COOKIE_JAR=$(mktemp)
trap "rm -f $COOKIE_JAR" EXIT

curl_server "/dev/login?user=foo" -X POST -c "$COOKIE_JAR" -o /dev/null -s

echo "--- /dashboard authed (should be 200) ---"
curl_server "/dashboard" -b "$COOKIE_JAR" --max-redirs 0 -o /dev/null -w "status=%{http_code}\n" -s

echo "--- POST /auth/logout ---"
curl_server "/auth/logout" -X POST -b "$COOKIE_JAR" -c "$COOKIE_JAR" --max-redirs 0 \
    -o /dev/null -w "status=%{http_code}\n" -s
{ curl_server "/auth/logout" -X POST --max-redirs 0 -s -D - -o /dev/null || true; } \
  | grep -i '^location:' | sed -E 's/\r$//' | head -1

echo "--- /dashboard after logout (back to redirect) ---"
{ curl_server "/dashboard" -b "$COOKIE_JAR" --max-redirs 0 -o /dev/null -w "status=%{http_code}\n" -s || true; }
