#!/usr/bin/env bash
# `ppz await --json` emits a sentinel arrival event on stdout BEFORE
# the JSON envelopes from the drain, so a downstream consumer can
# correlate {messages} with the pipe they came from without parsing
# the human banner.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a send chat.inbox "json payload" >/dev/null
wait_for 20 "ppz_a ls | grep -q 'json payload'" >/dev/null

OUT=$(ppz_a await --json chat.inbox 2>/dev/null)
# First line must be the arrival event.
echo "$OUT" | sed -n 1p | jq -c '{event, pipe}'
# Second line must be a JSON envelope carrying the payload.
echo "$OUT" | sed -n 2p | jq -r '.payload'
