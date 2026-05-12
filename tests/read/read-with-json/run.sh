#!/usr/bin/env bash
# --json switches output to NDJSON envelopes (one JSON object per line).
# Pipe through jq with a stable projection so we don't depend on UUID
# / RFC3339 values.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create chat >/dev/null
ppz_a send chat.inbox "hello" >/dev/null
ppz_a send chat.inbox "world" >/dev/null
wait_for 20 "ppz_a ls | grep -q world" >/dev/null

ppz_a read chat.inbox --json | jq -c '{sender, subject, payload}'
