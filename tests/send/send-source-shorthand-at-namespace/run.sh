#!/usr/bin/env bash
# Phase 1.5.2: `ppz send LEAF "msg"` shorthand falls back to LEAF.inbox
# when LEAF is a source. Pre-1.5.2 fallback only tested with LEAF at
# root manifold. When LEAF is a source at a NON-root manifold, the
# fallback must still resolve to <manifold>.LEAF.inbox.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a set namespace pixel >/dev/null
ppz_a source create boris >/dev/null

err=$(mktemp)
ppz_a send boris "shorthand-at-ns" 2>"$err"
echo "send-exit=$?"

# Normalised line: id collapsed, byte-count collapsed.
grep -oE '^sent id=[a-f0-9]{8} to=[^ ]+ bytes=[0-9]+$' "$err" | head -1 \
  | sed -E 's/id=[a-f0-9]{8}/id=ID8/; s/bytes=[0-9]+/bytes=N/'

# Verify the payload landed on pixel.boris.inbox specifically.
ppz_a reread pixel.boris.inbox -l 1 --json | jq -r .payload
rm -f "$err"
