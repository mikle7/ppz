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
ppz_a send --from pubsub boris "shorthand-at-ns" 2>"$err"
echo "send-exit=$?"

# Normalised send line: id collapsed, byte-count collapsed. The
# `to=pixel.boris.inbox` substring is the load-bearing assertion — it
# proves the shorthand resolved through the session's current_namespace
# and routed to the source at the right manifold.
grep -oE '^sent id=[a-f0-9]{8} to=[^ ]+ bytes=[0-9]+$' "$err" | head -1 \
  | sed -E 's/id=[a-f0-9]{8}/id=ID8/; s/bytes=[0-9]+/bytes=N/'

# Cross-check via ls — the BUFFERED count for pixel.boris.inbox should
# be 1 after the send lands. Locks in that the message physically
# arrived at the manifolded subject, not just that the CLI printed the
# right destination.
wait_for 20 "ppz_a ls | ls_normalize | grep -q '^pixel\.boris\.inbox 1 1'" >/dev/null
ppz_a ls 2>/dev/null | ls_normalize | awk '$1 == "pixel.boris.inbox" {print $1, $2, $3}'
rm -f "$err"
