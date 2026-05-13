#!/usr/bin/env bash
# When a server-touching call (ls / send / read / ...) returns
# E_INVALID_API_KEY — typically because the key was rotated or revoked
# server-side after login — the daemon caches that observation and
# `ppz status` reports the cached "authentication error" state.
#
# Without this, status keeps reporting "logged in" with the now-bad
# key and the user has no idea why their other commands are failing.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

# Simulate the key being revoked server-side by rewriting the stored
# api_key to a value the server doesn't recognise. The daemon polls
# the credential file every 200ms (file-watcher), so wait briefly.
sed -i 's/"api_key":"[^"]*"/"api_key":"bogus-revoked-key"/' "$PPZ_DAEMON_A_HOME/credentials"
sleep 0.5

echo "--- ls fails (server rejects the revoked key) ---"
ppz_a ls 2>&1 | grep '^error:' || true

echo "--- status reflects the cached invalid state ---"
ppz_a status
