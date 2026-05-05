#!/usr/bin/env bash
# RED → GREEN: source create with an OAuth bearer must respect the
# daemon's switched org (?org=<id>), not silently fall back to
# FirstOwnedOrgFor.
#
# Repro: foo owns alpha (auto-created on signup) AND we'll create a
# second org "gamma". After switching to gamma, POST /api/v1/sources
# must land in gamma, not alpha.
. /tests/lib/common.sh

TOKEN="ppz_oauth_test_foo_cccccccccccccccccccccccccccccccccccccccccccccc"
HASH=$(printf '%s' "$TOKEN" | sha256sum | awk '{print $1}')

PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -v ON_ERROR_STOP=1 -tAc "
  INSERT INTO oauth_tokens (id, user_id, token_hash, token_prefix, expires_at, created_at)
  SELECT '33333333-3333-3333-3333-333333333333', u.id,
         '$HASH', 'ppz_oaut', now() + interval '1 day', now()
    FROM users u WHERE u.username = 'foo'
  ON CONFLICT (token_hash) DO NOTHING
" >/dev/null

# Create gamma owned by foo via the API.
curl_server "/api/v1/orgs" -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"gamma"}' \
  -o /dev/null
gamma_id=$(PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc "
  SELECT id FROM organisations WHERE name = 'gamma'
")

# foo also owns alpha (per seed). Without ?org= the server would pick
# alpha (FirstOwnedOrgFor by created_at). We pass ?org=<gamma> and
# expect the source to land in gamma.
curl_server "/api/v1/sources?org=$gamma_id" -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"handle":"my-source"}' \
  -o /dev/null -w "create_status:%{http_code}\n"

PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc "
  SELECT s.handle, o.name FROM sources s
    JOIN organisations o ON o.id = s.organisation_id
   WHERE s.handle = 'my-source'
" | sed 's/|/ org=/'
