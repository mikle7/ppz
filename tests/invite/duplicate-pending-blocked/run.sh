#!/usr/bin/env bash
# Two invites to the same (org, username) → second one fails.
. /tests/lib/common.sh

auth_as foo

curl_server "/orgs/alpha/invites" -X POST -d "username=alice" -o /dev/null -w "first:%{http_code}\n"
curl_server "/orgs/alpha/invites" -X POST -d "username=alice" -o /dev/null -w "second:%{http_code}\n"

PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc "
  SELECT count(*) FROM invites WHERE invitee_username = 'alice' AND status = 'pending'
"
