#!/usr/bin/env bash
# Phase 2 regression: a session cookie HMAC-signed for a user that
# no longer exists in the DB must NOT pass the gate.
#
# Repro from real life: dev rig torn down + recreated; browser still
# held the cookie from the previous DB. The cookie's HMAC verified
# (same SESSION_KEY) but the user_id pointed at nothing → FK
# violation later when the route tried to insert FK-typed rows.
#
# Required behaviour: middleware should treat the cookie as invalid
# and 302 to /login, forcing a fresh OAuth round-trip.
#
# Self-contained: we INSERT a throwaway user, mint a cookie for them,
# then DELETE — that way the test doesn't rely on (or destroy) the
# seeded foo/bar fixtures, and re-runs cleanly without server reboot.
. /tests/lib/common.sh

THROWAWAY="cookie-stale-test-user"
THROWAWAY_ID="11111111-1111-1111-1111-111111111111"

# Insert (idempotent — re-running before reset.sh kicks in shouldn't fail).
PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -v ON_ERROR_STOP=1 -tAc "
  INSERT INTO users (id, username, email, mode)
  VALUES ('$THROWAWAY_ID', '$THROWAWAY', '$THROWAWAY@local', 'internal')
  ON CONFLICT (username) DO NOTHING
" >/dev/null

# Mint the session cookie for the throwaway.
COOKIE_JAR=$(mktemp)
trap "rm -f $COOKIE_JAR" EXIT
curl_server "/dev/login?user=$THROWAWAY" -X POST -c "$COOKIE_JAR" -o /dev/null -s

echo "--- sanity: cookie works while user exists ---"
curl_server "/dashboard" -b "$COOKIE_JAR" --max-redirs 0 \
    -o /dev/null -w "status=%{http_code}\n" -s

echo ""
echo "--- delete the user from the DB out from under the cookie ---"
PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -v ON_ERROR_STOP=1 -tAc \
    "DELETE FROM users WHERE username = '$THROWAWAY'" >/dev/null
echo "deleted=true"

echo ""
echo "--- /dashboard with the now-stale cookie → must redirect to /login ---"
curl_server "/dashboard" -b "$COOKIE_JAR" --max-redirs 0 \
    -o /dev/null -w "status=%{http_code}\n" -s
{ curl_server "/dashboard" -b "$COOKIE_JAR" --max-redirs 0 -s -D - -o /dev/null || true; } \
  | grep -i '^location:' | sed -E 's/\r$//' | head -1
