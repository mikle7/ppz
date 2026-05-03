#!/usr/bin/env bash
. /tests/lib/common.sh

# /api/v1/* must reach ppz-server. Without an API key, ppz-server
# returns 401 with the standard error envelope ({"error":{"code":
# "E_INVALID_API_KEY", ...}}). If routing instead served the static
# site or a Caddy 404, neither status nor body would match.
status=$(curl -sS -o /tmp/body -w '%{http_code}' "$PPZ_PUBLIC_URL/api/v1/sources")
printf 'status=%s\n' "$status"
if grep -q 'E_INVALID_API_KEY' /tmp/body; then echo 'envelope=present'; else echo 'envelope=absent'; fi
