#!/usr/bin/env bash
# Integration: full daemon-side flow after `ppz org switch`.
#
# Scenario:
#   1. Daemon A logs in as foo (OAuth) → lands in alpha by default.
#   2. Daemon A creates gamma + switches to it.
#   3. Daemon A creates source x in gamma + broadcasts "hello".
#   4. Daemon B logs in as foo, switches to gamma, reads x.broadcast.
#   5. Daemon B sees "hello".
#
# Exercises: daemon's ?org= stamping (callServer), per-org NATS
# reconnection on switch, and that subscribers in the same org see
# messages published before they connected.
. /tests/lib/common.sh

TOKEN="ppz_oauth_test_foo_dddddddddddddddddddddddddddddddddddddddddddddd"
HASH=$(printf '%s' "$TOKEN" | sha256sum | awk '{print $1}')

PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -v ON_ERROR_STOP=1 -tAc "
  INSERT INTO oauth_tokens (id, user_id, token_hash, token_prefix, expires_at, created_at)
  SELECT '44444444-4444-4444-4444-444444444444', u.id,
         '$HASH', 'ppz_oaut', now() + interval '1 day', now()
    FROM users u WHERE u.username = 'foo'
  ON CONFLICT (token_hash) DO NOTHING
" >/dev/null

# Daemon A: login → create gamma → switch → create source → broadcast.
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$TOKEN" >/dev/null
ppz_a org create gamma >/dev/null
ppz_a org switch gamma >/dev/null
ppz_a source create x >/dev/null

# Verify the source is in gamma, not alpha (regression guard for the
# requireAPIKey ?org= fix).
PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc "
  SELECT o.name FROM sources s
    JOIN organisations o ON o.id = s.organisation_id
   WHERE s.handle = 'x'
" | sed 's/^/source_x_org: /'

ppz_a broadcast "hello from gamma" >/dev/null

# Give NATS a moment to durably accept the publish before B subscribes.
sleep 0.3

# Daemon B: login (lands in foo's first owned = alpha) → switch to
# gamma → read.
ppz_b daemon login "$PPZ_SERVER_URL" -apikey "$TOKEN" >/dev/null
ppz_b org switch gamma >/dev/null

# Read with --raw so the output is just the payload (echo wraps it
# so we always get a trailing newline regardless of whether --raw
# emits one).
echo "$(ppz_b read x.broadcast --raw 2>/dev/null)"
