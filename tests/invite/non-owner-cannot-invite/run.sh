#!/usr/bin/env bash
# bar is a member of alpha but NOT the owner. POST to invites must 403.
. /tests/lib/common.sh

auth_as bar

curl_server "/orgs/alpha/invites" -X POST -d "username=alice" -o /dev/null -w "status:%{http_code}\n"

# Verify nothing was inserted.
PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc "
  SELECT count(*) FROM invites WHERE invitee_username = 'alice'
"
