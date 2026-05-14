#!/usr/bin/env bash
# /login is the auth-mode dispatcher. Under the default
# PPZ_SERVER_AUTH_MODE=none (the deployment shape OSS tests run
# against), it renders the upgrade-path informational panel.
. /tests/lib/common.sh

page="$(curl_server "/login")"

echo "--- login page identifier ---"
printf '%s' "$page" | grep -oE -m 1 'data-page="login"'

echo "--- auth mode ---"
printf '%s' "$page" | grep -oE -m 1 'data-auth-mode="none"'

echo "--- upgrade path mentions PPZ_SERVER_AUTH_MODE ---"
printf '%s' "$page" | grep -oE -m 1 'PPZ_SERVER_AUTH_MODE'

echo "--- continue link ---"
printf '%s' "$page" | grep -oE -m 1 'Continue to dashboard'
