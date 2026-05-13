#!/usr/bin/env bash
. /tests/lib/common.sh

# Read-path companion to source-broadcast-survives-stale-operator.
# Catches a subtle case: lazy stream re-creation might publish-side
# work but consumer-side (read / ls) might still hit the old/missing
# stream until the next access.
#
# Pre-stale messages are lost — JetStream is transient state per
# DEPLOYMENT.md and account reprovisioning rebuilds streams empty.
# The assertion is that POST-stale messages publish + read cleanly,
# i.e. the stream genuinely exists and is consumable in the new
# account namespace.

HANDLE="recovery-read"
KEY=$(key_alpha)

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$KEY" >/dev/null
ppz_a source create "$HANDLE" >/dev/null 2>&1 || true
ppz_a set handle "$HANDLE" >/dev/null
ppz_a send "$HANDLE.inbox" "before-stale" >/dev/null
wait_for 20 "ppz_a ls | grep -q before-stale" >/dev/null
echo "before-readable=$(ppz_a ls | grep -oE 'before-stale' | head -1)"

sim=$(curl -sS -o /dev/null -w '%{http_code}' \
  -X POST "$PPZ_SERVER_URL/api/v1/admin/simulate-stale-operator" \
  -H 'Content-Type: application/json' \
  -d "{\"api_key\":\"$KEY\"}")
echo "simulate=$sim"

# Account is freshly minted post-simulate; lazy stream provisioning has
# rebuilt empty streams under the new namespace. The "before-stale"
# message lived in the *previous* account namespace and is gone (this
# is the per-design transient JetStream behaviour). A new send
# must round-trip cleanly through the rebuilt streams.
ppz_a send "$HANDLE.inbox" "after-stale" >/dev/null
wait_for 20 "ppz_a ls | grep -q after-stale" >/dev/null
echo "after-readable=$(ppz_a ls | grep -oE 'after-stale' | head -1)"
