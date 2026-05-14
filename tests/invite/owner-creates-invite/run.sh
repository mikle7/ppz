#!/usr/bin/env bash
# foo (owner of alpha) invites alice → creates a pending row.
. /tests/lib/common.sh

# alice doesn't exist yet — that's the documented Phase 4 behaviour
# (invite by username works even before the invitee has signed up).
auth_as foo

curl_server "/accounts/alpha/invites" -X POST -d "username=alice" -o /dev/null -w "create_status:%{http_code}\n"

PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc "
  SELECT invitee_username, status FROM invites
   WHERE invitee_username = 'alice'
" | sed 's/|/ /'
