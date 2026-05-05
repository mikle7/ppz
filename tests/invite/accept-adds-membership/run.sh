#!/usr/bin/env bash
# alice accepts the invite — becomes an organisation_members row.
. /tests/lib/common.sh

PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -v ON_ERROR_STOP=1 -tAc "
  INSERT INTO users (id, username, email, mode)
  VALUES ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa','alice','alice@local','internal')
  ON CONFLICT (username) DO NOTHING
" >/dev/null

auth_as foo
curl_server "/orgs/alpha/invites" -X POST -d "username=alice" -o /dev/null

invite_id=$(PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc "
  SELECT id FROM invites WHERE invitee_username = 'alice' AND status = 'pending' LIMIT 1
")

auth_as alice
curl_server "/invites/$invite_id/accept" -X POST -o /dev/null -w "accept_status:%{http_code}\n"

PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc "
  SELECT status FROM invites WHERE id = '$invite_id'
" | sed 's/^/invite_status: /'

PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc "
  SELECT count(*) FROM organisation_members m
    JOIN organisations o ON o.id = m.organisation_id
    JOIN users u ON u.id = m.user_id
   WHERE o.name = 'alpha' AND u.username = 'alice'
" | sed 's/^/membership_rows: /'
