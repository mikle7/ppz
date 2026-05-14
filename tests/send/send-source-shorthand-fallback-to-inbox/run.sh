#!/usr/bin/env bash
# Phase 1.5.1 send resolution: `ppz send LEAF "msg"` tries uncollared
# first, falls back to legacy `LEAF.inbox` if LEAF is a source. Tests
# the FALLBACK leg: source `foo` exists, no uncollared `foo` → send
# resolves to foo.inbox. With the collision rule preventing both
# shapes from coexisting, the fallback is unambiguous.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a source create foo >/dev/null
ppz_a unset handle >/dev/null

err=$(mktemp)
ppz_a send foo "messaging shorthand" 2>"$err"
echo "send-exit=$?"
grep -oE '^sent id=[a-f0-9]{8} to=[^ ]+ bytes=[0-9]+$' "$err" | head -1 \
  | sed -E 's/id=[a-f0-9]{8}/id=ID8/; s/bytes=[0-9]+/bytes=N/'

ppz_a reread foo.inbox -l 1 --json | jq -r '.payload'
rm -f "$err"
