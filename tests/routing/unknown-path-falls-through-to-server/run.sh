#!/usr/bin/env bash
. /tests/lib/common.sh

# An unknown path (not a marketing route, not a known ppz-server
# handler) must fall through to ppz-server's mux, which returns 404.
# Crucially, the response must NOT be the static site's catch-all
# (file_server has no fallback document by default, but a future
# /404.html in site/dist would change that — guard against it).
status=$(curl -sS -o /tmp/body -w '%{http_code}' "$PPZ_PUBLIC_URL/this-path-does-not-exist-anywhere")
printf 'status=%s\n' "$status"
# ppz-server's stdlib mux 404 is a short text response, no HTML.
# A static-site 404 (if one existed) would have <!DOCTYPE html.
if grep -qi '<!DOCTYPE html' /tmp/body; then echo 'looks-like-static=true'; else echo 'looks-like-static=false'; fi
