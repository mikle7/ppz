#!/usr/bin/env bash
# bar is already a member of alpha. Inviting bar → 409.
. /tests/lib/common.sh

auth_as foo

curl_server "/orgs/alpha/invites" -X POST -d "username=bar" -o /dev/null -w "status:%{http_code}\n"

PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc "
  SELECT count(*) FROM invites WHERE invitee_username = 'bar'
"
