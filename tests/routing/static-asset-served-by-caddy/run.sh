#!/usr/bin/env bash
. /tests/lib/common.sh

# /static/* is the marketing assets prefix (logo, future CSS/JS/images).
# Caddy serves it via file_server from /srv/pipescloud-site/static/.
# Intentionally distinct from ppz-server's /assets/* (embedded GUI
# assets) — no routing collision possible.
status=$(curl -sS -o /tmp/body -w '%{http_code}' "$PPZ_PUBLIC_URL/static/logo.png")
printf 'status=%s\n' "$status"
if [ -s /tmp/body ]; then echo 'body=non-empty'; else echo 'body=empty'; fi
