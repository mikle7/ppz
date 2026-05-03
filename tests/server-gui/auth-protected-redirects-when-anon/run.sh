#!/usr/bin/env bash
# /dashboard is now session-gated. Anonymous (no cookie) requests must
# 302 to /login?next=/dashboard so the browser walks the user through
# OAuth and ends up at /dashboard afterwards.
. /tests/lib/common.sh

echo "--- /dashboard without cookie ---"
{ curl_server "/dashboard" --max-redirs 0 -o /dev/null -w "status=%{http_code}\n" -s || true; }
{ curl_server "/dashboard" --max-redirs 0 -s -D - -o /dev/null || true; } \
  | grep -i '^location:' | sed -E 's/\r$//' | head -1

echo ""
echo "--- /orgs/alpha/pipes without cookie (deep link preserves next=) ---"
{ curl_server "/orgs/alpha/pipes" --max-redirs 0 -o /dev/null -w "status=%{http_code}\n" -s || true; }
{ curl_server "/orgs/alpha/pipes" --max-redirs 0 -s -D - -o /dev/null || true; } \
  | grep -i '^location:' | sed -E 's/\r$//' | head -1
