#!/usr/bin/env bash
. /tests/lib/common.sh

# /healthz must reach ppz-server (not a static-site 404 or Caddy default).
# ppz-server's healthz returns the literal string "ok" with 200.
# Caddy itself never produces this exact body — so a match here
# proves the request was proxied to the right upstream.
status=$(curl -sS -o /tmp/body -w '%{http_code}' "$PPZ_PUBLIC_URL/healthz")
body=$(cat /tmp/body)
printf 'status=%s\n' "$status"
printf 'body=%s\n' "$body"
