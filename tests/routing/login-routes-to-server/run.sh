#!/usr/bin/env bash
. /tests/lib/common.sh

# /login is ppz-server's GUI login page. Marketing must NOT claim it
# (the marketing /signup might exist later, but /login stays with the
# server because that's where the OAuth flow originates).
status=$(curl -sS -o /tmp/body -w '%{http_code}' "$PPZ_PUBLIC_URL/login")
printf 'status=%s\n' "$status"
if grep -qi '<!DOCTYPE html' /tmp/body; then echo 'doctype=present'; else echo 'doctype=absent'; fi
# Login page renders a "Sign in with GitHub" affordance. Catching any
# of those markers proves we hit ppz-server's login handler, not a
# marketing /login page.
if grep -qi 'github' /tmp/body; then echo 'github-marker=present'; else echo 'github-marker=absent'; fi
