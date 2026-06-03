#!/usr/bin/env bash
# `subs read --json` must emit a clean JSONL stream (one envelope per line);
# the `=== <target> ===` banner would make it unparseable.
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
export PPZ_SESSION=mysh
ppz_a source create room-a >/dev/null
ppz_a subs add room-a.inbox >/dev/null
ppz_a send room-a.inbox from-a >/dev/null
wait_for 20 "ppz_a ls | grep -q from-a" >/dev/null
out=$(ppz_a subs read --json 2>/dev/null)
echo "json-separator-lines=$(printf '%s\n' "$out" | grep -c '^=== ' || true)"
echo "payload=$(printf '%s\n' "$out" | jq -r 'select(.payload!=null) | .payload' 2>/dev/null | head -1)"
