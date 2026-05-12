#!/usr/bin/env bash
. /tests/lib/common.sh

# Production scenario reproduced on 2026-05-03: an infra-deploy churned
# the operator JWT (different operator key signing on each pulumi up
# in the current natsauth.go). The org's existing nats_account_jwt
# in postgres was signed by the prior operator → embedded NATS rejects
# every connection into that account with "Authorization Violation"
# until ppz-server reprovisions.
#
# Compose simulates this faithfully via POST /api/v1/admin/simulate-
# stale-operator (dev-gated): mints a fresh operator + account
# material in-memory, signs an account JWT with the *fake* operator,
# overwrites the org's postgres row, drops the AccountPool cache.
# Postgres looks self-consistent; NATS doesn't trust the chain.
#
# Fix-spec:
#   1. AccountPool.Get's openAccount detects Authorization Violation
#      and triggers provisionAccount (mint fresh account JWT signed
#      by current operator, push to resolver), then retries openAccount.
#   2. provisionAccount, after pushing, walks db.ListSourcesForOrg
#      and ensures each source's auto-streams (broadcast/stdin/stdout)
#      exist in the new account namespace. Existing → no-op. Missing
#      → create.
#
# Test passes when the second broadcast still returns "sent" against
# the same handle the user originally created.

HANDLE="recovery-broadcast"
KEY=$(key_alpha)

# 1. Setup: log in alpha, create a source, baseline send must work.
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$KEY" >/dev/null
ppz_a terminal create "$HANDLE" >/dev/null 2>&1 || true
ppz_a set handle "$HANDLE" >/dev/null
out1=$(ppz_a send "$HANDLE.inbox" "before" 2>&1)
echo "before=$(echo "$out1" | grep -oE '^sent\b|^error: E_[A-Z_]+' | head -1)"

# 2. Faithfully reproduce the prod state — operator now untrusted by
#    the running NATS server, postgres row left intact (matches what
#    an infra-deploy that rotated the operator JWT does).
sim=$(curl -sS -o /dev/null -w '%{http_code}' \
  -X POST "$PPZ_SERVER_URL/api/v1/admin/simulate-stale-operator" \
  -H 'Content-Type: application/json' \
  -d "{\"api_key\":\"$KEY\"}")
echo "simulate=$sim"

# 3. Re-send against the SAME handle — must still succeed via
#    auto-recovery (Authorization Violation → provision → lazy stream
#    re-create → publish).
out2=$(ppz_a send "$HANDLE.inbox" "after" 2>&1)
echo "after=$(echo "$out2" | grep -oE '^sent\b|^error: E_[A-Z_]+' | head -1)"
