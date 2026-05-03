#!/usr/bin/env bash
. /tests/lib/common.sh

status=$(curl -sS -o /tmp/body -w '%{http_code}' "$PPZ_PUBLIC_URL/pricing")
printf 'status=%s\n' "$status"
if grep -qi '<!DOCTYPE html' /tmp/body; then echo 'doctype=present'; else echo 'doctype=absent'; fi
