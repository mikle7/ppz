#!/usr/bin/env bash
# Members in v1: an owner can add a known user as a member of their
# org and can remove members (but NOT the owner — that's transfer-
# ownership territory, deferred to v2). v2 will replace direct
# add-by-username with email-invite; v1's "Add member" form is the
# functional equivalent so the spec exists end-to-end today.
#
# Open spec point — currently testing direct add-by-username via
# POST /accounts/<id>/members. If the intent was that v1 has NO member
# add (owner-only until v2 invite-by-email lands), say so and
# we'll cut the add path from this test.
. /tests/lib/common.sh
auth_as_foo

# Seed: two users, one org owned by alice.
curl_server "/users" -X POST \
  --data-urlencode 'username=alice' \
  --data-urlencode 'email=alice@example.com' \
  --data-urlencode 'mode=internal' >/dev/null
curl_server "/users" -X POST \
  --data-urlencode 'username=bob' \
  --data-urlencode 'email=bob@example.com' \
  --data-urlencode 'mode=internal' >/dev/null
alice_id="$(PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc \
  "SELECT id FROM users WHERE username = 'alice'")"
bob_id="$(PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc \
  "SELECT id FROM users WHERE username = 'bob'")"
curl_server "/accounts" -X POST \
  --data-urlencode 'name=wonder' \
  --data-urlencode "owner_user_id=$alice_id" >/dev/null
wonder_id="$(PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc \
  "SELECT id FROM accounts WHERE name = 'wonder'")"

echo "--- add bob as member ---"
curl_server "/accounts/$wonder_id/members" -X POST \
  --data-urlencode "user_id=$bob_id" \
  -o /dev/null -w 'http=%{http_code}\n'

echo "--- bob is now a member of wonder ---"
PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc \
  "SELECT u.username FROM account_members m
     JOIN users u ON m.user_id = u.id
    WHERE m.account_id = '$wonder_id'"

echo "--- removing bob (regular member) succeeds ---"
curl_server "/accounts/$wonder_id/members/$bob_id/remove" -X POST \
  -o /dev/null -w 'http=%{http_code}\n'

echo "--- members table is empty again ---"
PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc \
  "SELECT COUNT(*)::text FROM account_members WHERE account_id = '$wonder_id'"

echo "--- removing alice (owner) is rejected ---"
curl_server "/accounts/$wonder_id/members/$alice_id/remove" -X POST \
  -o /dev/null -w 'http=%{http_code}\n'

echo "--- alice still owns wonder ---"
PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc \
  "SELECT u.username FROM accounts o
     JOIN users u ON o.owner_user_id = u.id
    WHERE o.name = 'wonder'"
