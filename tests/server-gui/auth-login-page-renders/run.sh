#!/usr/bin/env bash
# /login is the new gateway page. It must render a "Continue with GitHub"
# button (or anchor) so anonymous visitors have a clear path to OAuth.
. /tests/lib/common.sh

page="$(curl_server "/login")"

echo "--- login page identifier ---"
printf '%s' "$page" | grep -oE -m 1 'data-page="login"'

echo "--- GitHub auth link present ---"
printf '%s' "$page" | grep -oE -m 1 'href="/auth/github/start[^"]*"'

echo "--- CTA copy ---"
printf '%s' "$page" | grep -oE -m 1 'Continue with GitHub'
