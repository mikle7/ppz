#!/usr/bin/env bash
# The e2e seed includes two ready-to-use internal users — `foo` and
# `bar` — so member-management tests can add/remove without first
# spinning up `POST /users` boilerplate. Their IDs land in
# /seed/user-foo.txt and /seed/user-bar.txt, mirroring how alpha
# and beta org IDs are surfaced.
. /tests/lib/common.sh
auth_as_foo

echo "--- seed user files exist + non-empty ---"
[[ -s /seed/user-foo.txt ]] && echo "user-foo.txt=present" || echo "user-foo.txt=missing"
[[ -s /seed/user-bar.txt ]] && echo "user-bar.txt=present" || echo "user-bar.txt=missing"

echo "--- both users are in the database with mode=internal ---"
PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc \
  "SELECT username || '|' || mode FROM users WHERE username IN ('foo','bar') ORDER BY username"

echo "--- the seed files match the database ids ---"
foo_seed="$(cat /seed/user-foo.txt)"
bar_seed="$(cat /seed/user-bar.txt)"
foo_db="$(PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc "SELECT id FROM users WHERE username = 'foo'")"
bar_db="$(PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc "SELECT id FROM users WHERE username = 'bar'")"
[[ "$foo_seed" == "$foo_db" ]] && echo "foo-id-matches=true"  || echo "foo-id-matches=false"
[[ "$bar_seed" == "$bar_db" ]] && echo "bar-id-matches=true"  || echo "bar-id-matches=false"

echo "--- seeded memberships: foo→alpha, bar→beta ---"
PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc \
  "SELECT u.username || '→' || o.name
     FROM account_members m
     JOIN users u         ON u.id = m.user_id
     JOIN accounts o ON o.id = m.account_id
    ORDER BY o.name, u.username"
