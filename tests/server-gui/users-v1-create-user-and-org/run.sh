#!/usr/bin/env bash
# v1 user + org creation: POST /users creates a user (username, email,
# mode=internal). POST /orgs accepts an optional owner_user_id and
# wires the new org to that user — omitting it defaults to the
# currently-authenticated session user, so the creator immediately
# sees their new org on /dashboard.
. /tests/lib/common.sh
auth_as_foo

echo "--- create user alice ---"
curl_server "/users" -X POST \
  --data-urlencode 'username=alice' \
  --data-urlencode 'email=alice@example.com' \
  --data-urlencode 'mode=internal' \
  -o /dev/null -w 'http=%{http_code}\n'

echo "--- alice now appears in the database ---"
PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc \
  "SELECT username || '|' || email || '|' || mode FROM users WHERE username = 'alice'"

# Capture alice's user_id for the org-create POST.
alice_id="$(PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc \
  "SELECT id FROM users WHERE username = 'alice'")"

echo "--- create org wonder owned by alice ---"
curl_server "/orgs" -X POST \
  --data-urlencode 'name=wonder' \
  --data-urlencode "owner_user_id=$alice_id" \
  -o /dev/null -w 'http=%{http_code}\n'

echo "--- wonder is owned by alice ---"
PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc \
  "SELECT u.username FROM accounts o
     JOIN users u ON o.owner_user_id = u.id
    WHERE o.name = 'wonder'"

echo "--- POST /orgs with only a name → owner defaults to the authed session user ---"
curl_server "/orgs" -X POST --data-urlencode 'name=plain' \
  -o /dev/null -w 'http=%{http_code}\n'
PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc \
  "SELECT u.username FROM accounts o
     JOIN users u ON o.owner_user_id = u.id
    WHERE o.name = 'plain'"
