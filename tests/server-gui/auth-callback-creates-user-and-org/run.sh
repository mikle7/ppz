#!/usr/bin/env bash
# Full OAuth round-trip with mock-github: anonymous user clicks
# "Continue with GitHub", we redirect to mock-github, mock redirects
# back to /auth/github/callback with a code, ppz-server exchanges the
# code, fetches the user, creates a new users row + a new
# organisation owned by them, sets a session cookie, redirects to
# /dashboard.
. /tests/lib/common.sh

COOKIE_JAR=$(mktemp)
trap "rm -f $COOKIE_JAR" EXIT

echo "--- step 1: GET /auth/github/start should 302 to mock-github ---"
{ curl_server "/auth/github/start" --max-redirs 0 -c "$COOKIE_JAR" \
    -o /dev/null -w "status=%{http_code}\n" -s || true; }
{ curl_server "/auth/github/start" --max-redirs 0 -s -D - -o /dev/null || true; } \
  | grep -i '^location:' | sed -E 's/^[Ll]ocation:\s*//' | sed -E 's/\r$//' \
  | sed -E 's#^(http://[^/]+)/.*#\1#' \
  | head -1

echo ""
echo "--- step 2: follow the full flow, should land on /dashboard with 200 ---"
final_status=$(curl_server "/auth/github/start" -L -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
    -o /tmp/dash.html -s -w "%{http_code}")
echo "final_status=$final_status"

echo ""
echo "--- step 3: cookie set ---"
grep -q "ppz_session" "$COOKIE_JAR" && echo "session_cookie_present=true" || echo "session_cookie_present=false"

echo ""
echo "--- step 4: /me reflects the new GitHub user ---"
curl_server "/me" -b "$COOKIE_JAR" -s | grep -oE '"username":"[^"]+"|"github_id":[0-9]+'

echo ""
echo "--- step 5: org auto-created with the user as owner ---"
curl_server "/dashboard" -b "$COOKIE_JAR" -s | grep -oE 'data-org="gh-test-user"' | head -1
