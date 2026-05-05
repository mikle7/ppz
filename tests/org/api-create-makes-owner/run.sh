#!/usr/bin/env bash
# POST /api/v1/orgs creates the org with caller as owner.
. /tests/lib/common.sh

TOKEN="ppz_oauth_test_foo_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
HASH=$(printf '%s' "$TOKEN" | sha256sum | awk '{print $1}')

PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -v ON_ERROR_STOP=1 -tAc "
  INSERT INTO oauth_tokens (id, user_id, token_hash, token_prefix, expires_at, created_at)
  SELECT '22222222-2222-2222-2222-222222222222', u.id,
         '$HASH', 'ppz_oaut', now() + interval '1 day', now()
    FROM users u WHERE u.username = 'foo'
  ON CONFLICT (token_hash) DO NOTHING
" >/dev/null

curl_server "/api/v1/orgs" -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"gamma"}' \
  -o /dev/null -w "status:%{http_code}\n"

PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc "
  SELECT o.name, u.username FROM organisations o
    JOIN users u ON u.id = o.owner_user_id
   WHERE o.name = 'gamma'
" | sed 's/|/ owner=/'
