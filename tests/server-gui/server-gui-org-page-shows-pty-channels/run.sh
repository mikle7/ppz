#!/usr/bin/env bash
# A pty source exposes five pipes (heartbeat, inbox, stdctrl, stdin,
# stdout). The org page table must render one row per (source, pipe)
# for pty sources, and one row (pipe=inbox) for message sources. Each
# row gets data-source-row and data-source-pipe-link markers.
. /tests/lib/common.sh
auth_as_foo

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create regular >/dev/null
ppz_a terminal share pty-pipe -- true >/dev/null

PAGE=$(curl_server "/orgs/alpha")

echo "--- source-rows ---"
echo "$PAGE" \
  | grep -oE 'data-source-row="[^"]+"' \
  | sed -E 's/data-source-row="([^"]+)"/\1/' \
  | sed -E 's/:(just now|[0-9]+ (seconds?|minutes?|hours?|days?) ago):/:RELATIVE:/' \
  | sed -E 's|^([^:]+:heartbeat:RELATIVE):.*|\1:HEARTBEAT_JSON|' \
  | sed -E 's|:RELATIVE:\{&#34;.*|:RELATIVE:STDCTRL_JSON|' \
  | sort

echo "--- pipe-links ---"
echo "$PAGE" \
  | grep -oE 'data-source-pipe-link="[^"]+"' \
  | sed -E 's/data-source-pipe-link="([^"]+)"/\1/' \
  | sort
