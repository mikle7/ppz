#!/usr/bin/env bash
# Decline frees the (org, username) slot — owner can re-invite.
. /tests/lib/common.sh

PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -v ON_ERROR_STOP=1 -tAc "
  INSERT INTO users (id, username, email, mode)
  VALUES ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa','alice','alice@local','internal')
  ON CONFLICT (username) DO NOTHING
" >/dev/null

auth_as foo
curl_server "/accounts/alpha/invites" -X POST -d "username=alice" -o /dev/null

invite_id=$(PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc "
  SELECT id FROM invites WHERE invitee_username = 'alice' ORDER BY created_at DESC LIMIT 1
")

auth_as alice
curl_server "/invites/$invite_id/decline" -X POST -o /dev/null -w "decline_status:%{http_code}\n"

# Re-invite — must succeed (no pending row, declined doesn't block).
auth_as foo
curl_server "/accounts/alpha/invites" -X POST -d "username=alice" -o /dev/null -w "reinvite_status:%{http_code}\n"

# Final state: 1 declined + 1 pending row.
PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc "
  SELECT status, count(*) FROM invites
   WHERE invitee_username = 'alice'
   GROUP BY status
   ORDER BY status
" | sed 's/|/ /'
