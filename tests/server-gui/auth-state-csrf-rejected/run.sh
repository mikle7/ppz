#!/usr/bin/env bash
# CSRF defence: hitting /auth/github/callback with a state we never
# minted (or one already consumed) must be rejected. No user created,
# no cookie set, 400 Bad Request.
. /tests/lib/common.sh

COOKIE_JAR=$(mktemp)
trap "rm -f $COOKIE_JAR" EXIT

echo "--- bogus state, valid-looking code ---"
curl_server "/auth/github/callback?code=test_code&state=bogus-not-minted" \
    --max-redirs 0 -c "$COOKIE_JAR" -o /dev/null -w "status=%{http_code}\n" -s

echo ""
echo "--- no session cookie set ---"
grep -c "ppz_session" "$COOKIE_JAR" 2>/dev/null
true

echo ""
echo "--- replay: mint a state, use it, use it again — second attempt rejected ---"
START_LOC=$({ curl_server "/auth/github/start" --max-redirs 0 -s -D - -o /dev/null || true; } \
              | grep -i '^location:' | head -1)
STATE=$(echo "$START_LOC" | grep -oE 'state=[^&[:space:]]+' | cut -d= -f2)
echo "minted_state_present=$([[ -n "$STATE" ]] && echo true || echo false)"

# Step 2: complete the callback once (legit). Mock-github accepts any code.
{ curl_server "/auth/github/callback?code=test_code&state=$STATE" --max-redirs 0 \
    -o /dev/null -w "first_callback=%{http_code}\n" -s || true; }

# Step 3: replay the same state. Must be rejected.
curl_server "/auth/github/callback?code=test_code&state=$STATE" --max-redirs 0 \
    -o /dev/null -w "replay_callback=%{http_code}\n" -s
