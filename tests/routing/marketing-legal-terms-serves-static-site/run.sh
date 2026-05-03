#!/usr/bin/env bash
. /tests/lib/common.sh

# /legal/* is the catch-all marketing prefix for terms, privacy, etc.
# Caddy's `handle /legal/*` routes the whole subtree to file_server.
status=$(curl -sS -o /tmp/body -w '%{http_code}' "$PPZ_PUBLIC_URL/legal/terms")
printf 'status=%s\n' "$status"
if grep -qi '<!DOCTYPE html' /tmp/body; then echo 'doctype=present'; else echo 'doctype=absent'; fi
