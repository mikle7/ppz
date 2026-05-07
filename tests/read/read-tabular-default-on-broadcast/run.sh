#!/usr/bin/env bash
# v0.23.0 default `ppz read` against an inbox-shaped pipe (broadcast / inbox)
# renders three columns: HH:MM:SS, sender (`-` if empty), payload. This
# scenario locks the format. Scripts wanting the legacy bare payload-only
# stream pass --bare (covered by other read-* fixtures).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a broadcast -m "first message" >/dev/null
ppz_a broadcast -m "second message" >/dev/null
wait_for 20 "ppz_a ls | grep -q 'second message'" >/dev/null

# Default mode renders one row per message with the three-column shape.
# The timestamp varies per run; normalize.sh rewrites leading HH:MM:SS to
# the literal `HH:MM:SS` token so the diff stays stable.
ppz_a read chat.broadcast
