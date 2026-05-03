#!/usr/bin/env bash
. /tests/lib/common.sh

# GET / must reach the pipescloud marketing static site, NOT ppz-server's
# legacy GUI landing handler. The static site is served by Caddy's
# file_server from /srv/pipescloud-site (see compose/Caddyfile.compose
# and infra/userdata.sh.tpl).
status=$(curl -sS -o /tmp/body -w '%{http_code}' "$PPZ_PUBLIC_URL/")
printf 'status=%s\n' "$status"
if grep -qi '<!DOCTYPE html' /tmp/body; then echo 'doctype=present'; else echo 'doctype=absent'; fi
# Brand marker: the marketing site's pages must carry the word
# "pipescloud" somewhere visible (header, footer, or hero copy).
# ppz-server's legacy landing won't satisfy this. The exact phrasing
# is up to the implementer; the spec is "the marketing site
# identifies itself".
if grep -qi 'pipescloud' /tmp/body; then echo 'brand-marker=present'; else echo 'brand-marker=absent'; fi
