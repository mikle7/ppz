#!/usr/bin/env bash
# Users v1 seed contract: a placeholder user with mode=internal + a
# stable username (here "unauthenticated") exists, and the seeded
# alpha + beta organisations are owned by that user. Lets the e2e
# harness keep creating orgs/keys/sources without an OAuth flow,
# while still exercising the new "every org has an owner" invariant.
. /tests/lib/common.sh
auth_as_foo

echo "--- internal placeholder user exists ---"
PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc \
  "SELECT username || '|' || mode FROM users WHERE username = 'unauthenticated'"

echo "--- alpha and beta both have internal-mode owners ---"
# v2: alpha is now owned by foo (the auth-tests fixture); beta still
# owned by the unauthenticated placeholder. Both are mode=internal,
# which is what the contract actually pins.
PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc \
  "SELECT o.name || ' owned by ' || u.username || ' (' || u.mode || ')'
     FROM accounts o
     JOIN users u ON o.owner_user_id = u.id
    WHERE o.name IN ('alpha','beta')
    ORDER BY o.name"

echo "--- alpha's org page surfaces the owner ---"
org_id="$(cat /seed/org-alpha.txt)"
curl_server "/accounts/$org_id" \
  | grep -oE 'data-owner-username="[^"]+"' \
  | head -1
