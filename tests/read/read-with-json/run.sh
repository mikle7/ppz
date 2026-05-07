#!/usr/bin/env bash
# --json switches output to NDJSON envelopes (one JSON object per line).
# Pipe through jq with a stable projection so we don't depend on UUID
# / RFC3339 values.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a broadcast -m "hello" >/dev/null
ppz_a broadcast -m "world" >/dev/null
wait_for 20 "ppz_a ls | grep -q world" >/dev/null

ppz_a read chat.broadcast --json | jq -c '{sender, subject, payload}'
