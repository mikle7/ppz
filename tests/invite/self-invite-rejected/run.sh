#!/usr/bin/env bash
# foo invites foo → 400 ("cannot invite yourself")
. /tests/lib/common.sh

auth_as foo

curl_server "/orgs/alpha/invites" -X POST -d "username=foo" -o /dev/null -w "status:%{http_code}\n"

PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc "
  SELECT count(*) FROM invites WHERE invitee_username = 'foo'
"
