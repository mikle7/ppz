#!/usr/bin/env bash
# `ppz ls` after `org switch` must surface the switched org's
# sources, not the auto-org's. Regression guard for the requireAPIKey
# ?org= fix on the GET path.
. /tests/lib/common.sh

TOKEN="ppz_oauth_test_foo_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
HASH=$(printf '%s' "$TOKEN" | sha256sum | awk '{print $1}')

PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -v ON_ERROR_STOP=1 -tAc "
  INSERT INTO oauth_tokens (id, user_id, token_hash, token_prefix, expires_at, created_at)
  SELECT '55555555-5555-5555-5555-555555555555', u.id,
         '$HASH', 'ppz_oaut', now() + interval '1 day', now()
    FROM users u WHERE u.username = 'foo'
  ON CONFLICT (token_hash) DO NOTHING
" >/dev/null

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$TOKEN" >/dev/null

# Source in alpha (the auto-org foo lands in by default).
ppz_a source create alpha-only-src >/dev/null

# Switch to gamma; create a different source there.
ppz_a org create gamma >/dev/null
ppz_a org switch gamma >/dev/null
ppz_a source create gamma-only-src >/dev/null

# `ppz ls` should now show ONLY gamma's source.
ppz_a ls | ls_normalize | awk '{print $1}' | sort
