#!/usr/bin/env bash
# An API key (org-scoped, no user identity) can't create orgs — 403.
. /tests/lib/common.sh

curl_server "/api/v1/orgs" -X POST \
  -H "Authorization: Bearer $(key_alpha)" \
  -H "Content-Type: application/json" \
  -d '{"name":"shouldfail"}' \
  -o /dev/null -w "status:%{http_code}\n"

PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc "
  SELECT count(*) FROM organisations WHERE name = 'shouldfail'
"
