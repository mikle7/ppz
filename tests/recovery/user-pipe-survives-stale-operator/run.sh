#!/usr/bin/env bash
. /tests/lib/common.sh

# User-created pipes (ppz pipe create handle.name) live in postgres's
# `pipes` table — a different code path from the auto-provisioned
# inbox/stdin/stdout/stdctrl streams that come from `sources`. Lazy
# stream provisioning during account recovery must walk both tables,
# not just sources. This test pins that.

HANDLE="recovery-pipe-src"
PIPE="my-pipe"
KEY=$(key_alpha)

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$KEY" >/dev/null
ppz_a source create "$HANDLE" >/dev/null 2>&1 || true
ppz_a set handle "$HANDLE" >/dev/null
ppz_a pipe create "$PIPE" >/dev/null
out1=$(ppz_a send "$HANDLE.$PIPE" "before" 2>&1)
echo "before=$(echo "$out1" | grep -oE '^sent\b|^error: E_[A-Z_]+' | head -1)"

sim=$(curl -sS -o /dev/null -w '%{http_code}' \
  -X POST "$PPZ_SERVER_URL/api/v1/admin/simulate-stale-operator" \
  -H 'Content-Type: application/json' \
  -d "{\"api_key\":\"$KEY\"}")
echo "simulate=$sim"

out2=$(ppz_a send "$HANDLE.$PIPE" "after" 2>&1)
echo "after=$(echo "$out2" | grep -oE '^sent\b|^error: E_[A-Z_]+' | head -1)"
