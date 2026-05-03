#!/usr/bin/env bash
. /tests/lib/common.sh

# POST /oauth/device/code must reach ppz-server's device-flow handler.
# Caddy's static file_server doesn't accept POSTs (returns 405), and
# nothing on the marketing side claims this path. A 200 with a body
# containing device_code/user_code is unambiguous evidence the request
# reached ppz-server.
status=$(curl -sS -X POST -o /tmp/body -w '%{http_code}' "$PPZ_PUBLIC_URL/oauth/device/code")
printf 'status=%s\n' "$status"
if grep -q 'device_code\|user_code' /tmp/body; then echo 'device-payload=present'; else echo 'device-payload=absent'; fi
