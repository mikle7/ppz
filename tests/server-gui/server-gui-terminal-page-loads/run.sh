#!/usr/bin/env bash
# The GUI exposes a live terminal viewer at
#   /orgs/<slug>/sources/<handle>/terminal
# for any source whose .stdout has been written to (i.e. a shared
# terminal). The page hosts xterm.js, mounts it on #terminal, and opens
# a WebSocket against /terminal/ws for a binary stream of .stdout bytes.
#
# This test pins the page contract:
#   - returns 200
#   - includes the xterm.js + xterm.css assets from /static/
#   - has a #terminal mount point
#   - bootstraps a WebSocket connection to the right path
. /tests/lib/common.sh
auth_as_foo

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
# Wrap an explicit child so the test exits cleanly. The wrap creates the
# pty source + stdin/stdout pipes; the printf produces a .stdout chunk so
# the page has something to render.
ppz_a terminal share term1 -- printf "hello-from-share" >/dev/null
wait_for 20 "ppz_a read term1.stdout --json | jq -r '.payload' | grep -q hello-from-share" >/dev/null

PAGE=$(curl_server "/orgs/alpha/sources/term1/terminal")

echo "--- xterm assets referenced ---"
echo "$PAGE" | grep -oE '/assets/xterm\.(js|css)' | sort -u

echo "--- mount point ---"
echo "$PAGE" | grep -oE 'id="terminal"' | head -1

echo "--- WebSocket bootstrap path ---"
echo "$PAGE" \
  | grep -oE '/orgs/[a-z0-9-]+/sources/[a-z0-9-]+/terminal/ws' \
  | head -1
