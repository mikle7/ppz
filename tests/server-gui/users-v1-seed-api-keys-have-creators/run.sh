#!/usr/bin/env bash
# Seeded api keys are attributed to seeded users:
#
#   alpha-primary    → foo  (alpha owner)
#   alpha-secondary  → bar  (alpha member)
#   beta-primary     → bar  (beta member)
#
# Lets the e2e suite use the seed keys as deterministic stand-ins for
# real GitHub-mode users when asserting attribution behaviour.
. /tests/lib/common.sh

PGPASSWORD=ppz psql -h postgres -U postgres -d ppz -tAc \
  "SELECT k.label || '→' || u.username
     FROM api_keys k
     JOIN users u ON u.id = k.created_by_user_id
    WHERE k.label IN ('alpha-primary','alpha-secondary','beta-primary')
    ORDER BY k.label"
