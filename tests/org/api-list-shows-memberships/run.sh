#!/usr/bin/env bash
# GET /api/v1/orgs returns owner+member orgs for the bearer's user.
# bar is a non-owner member of alpha and beta (per seed).
. /tests/lib/common.sh

# Mint a test OAuth bearer for bar. The token table stores a SHA256
# of the plaintext; we compute it here so the bearer middleware
# accepts the value we curl with.
TOKEN="ppz_oauth_test_bar_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
HASH=$(printf '%s' "$TOKEN" | sha256sum | awk '{print $1}')

PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -v ON_ERROR_STOP=1 -tAc "
  INSERT INTO oauth_tokens (id, user_id, token_hash, token_prefix, expires_at, created_at)
  SELECT '11111111-1111-1111-1111-111111111111', u.id,
         '$HASH', 'ppz_oaut', now() + interval '1 day', now()
    FROM users u WHERE u.username = 'bar'
  ON CONFLICT (token_hash) DO NOTHING
" >/dev/null

curl_server "/api/v1/orgs" -H "Authorization: Bearer $TOKEN" -s \
  | jq -r '.orgs | sort_by(.name) | .[] | "\(.name) \(.role)"'
