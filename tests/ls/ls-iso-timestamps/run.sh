#!/usr/bin/env bash
# `ppz ls --iso` keeps the table format but shows RFC3339 timestamps in
# the LAST column instead of relative durations. Useful for agents that
# want the table layout but precise sortable timestamps.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a send chat.inbox "iso row" >/dev/null
wait_for 20 "ppz_a ls | grep -q 'iso row'" >/dev/null

# Normalise whitespace and the timestamp.
ppz_a ls --iso \
  | sed -E 's/[[:space:]]+/ /g' \
  | sed -E 's/[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})/TIMESTAMP/'
