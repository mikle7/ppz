#!/usr/bin/env bash
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create live >/dev/null

# Snapshot before any broadcast — payload should be null.
ppz-desktop --dump-state --ipc="$PPZ_DAEMON_A_SOCK" \
  | jq -r '.sources[] | select(.handle=="live") | .last_broadcast_payload'

ppz_a broadcast -m "live update" >/dev/null

# Wait until the desktop snapshot reflects the new payload.
wait_for 20 "ppz-desktop --dump-state --ipc=\"$PPZ_DAEMON_A_SOCK\" | jq -e '.sources[] | select(.handle==\"live\") | .last_broadcast_payload == \"live update\"' >/dev/null" >/dev/null

ppz-desktop --dump-state --ipc="$PPZ_DAEMON_A_SOCK" \
  | jq -r '.sources[] | select(.handle=="live") | .last_broadcast_payload'
