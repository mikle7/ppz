#!/usr/bin/env bash
. /tests/lib/common.sh

# Anonymous browser request to /dashboard must hit ppz-server's
# requireSession middleware, which 302s to /login?next=%2Fdashboard.
# Verifies (a) the request reaches ppz-server (Caddy didn't intercept),
# and (b) ppz-server's auth gate fires correctly through the proxy.
status=$(curl -sS -o /dev/null -w '%{http_code}' "$PPZ_PUBLIC_URL/dashboard")
location=$(curl -sS -o /dev/null -D - "$PPZ_PUBLIC_URL/dashboard" | grep -i '^Location:' | tr -d '\r' | awk '{print $2}')
printf 'status=%s\n' "$status"
printf 'location=%s\n' "$location"
