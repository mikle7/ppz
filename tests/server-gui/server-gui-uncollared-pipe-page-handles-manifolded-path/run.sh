#!/usr/bin/env bash
# Companion to server-gui-uncollared-pipe-page-lists-buffered-messages.
# That scenario covers an uncollared pipe at the root manifold
# (`/orgs/alpha/pipes/testroom`). This one exercises the manifolded
# shape — `/orgs/alpha/pipes/<manifold>.<leaf>` — where the handler
# has to split the dotted {pipe} segment on the LAST dot to recover
# (manifold, name). A future "split on first dot" regression would
# pass the root-manifold scenario but break here.
. /tests/lib/common.sh
auth_as_foo

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1

# Create an uncollared pipe at manifold "pixel" — bare LEAF with the
# namespace set is the Phase 1.5.1 path for uncollared-at-manifold
# creation. Sends use the same bare-leaf shape; namespace state
# routes them to the manifolded subject.
ppz_a set namespace pixel >/dev/null
ppz_a pipe create testroom >/dev/null
ppz_a send testroom "msg-1" >/dev/null
ppz_a send testroom "msg-2" >/dev/null
wait_for 20 "ppz_a ls | grep -q msg-2" >/dev/null

# URL segment is the full dotted "<manifold>.<leaf>" path the org
# pipes table surfaces — splitting that back into (pixel, testroom)
# happens server-side.
curl_server "/orgs/alpha/pipes/pixel.testroom" \
  | grep -oE 'data-message="[^"]+"' \
  | sed -E 's/data-message="([^"]+)"/\1/'
