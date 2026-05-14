#!/usr/bin/env bash
# bar (member of alpha, not owner) can't revoke alpha's invites.
. /tests/lib/common.sh

PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -v ON_ERROR_STOP=1 -tAc "
  INSERT INTO users (id, username, email, mode)
  VALUES ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa','alice','alice@local','internal')
  ON CONFLICT (username) DO NOTHING
" >/dev/null

auth_as foo
curl_server "/accounts/alpha/invites" -X POST -d "username=alice" -o /dev/null

invite_id=$(PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc "
  SELECT id FROM invites WHERE invitee_username = 'alice' AND status = 'pending'
")

auth_as bar
curl_server "/accounts/alpha/invites/$invite_id/revoke" -X POST -o /dev/null -w "status:%{http_code}\n"

PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc "
  SELECT status FROM invites WHERE id = '$invite_id'
" | sed 's/^/invite_status: /'
