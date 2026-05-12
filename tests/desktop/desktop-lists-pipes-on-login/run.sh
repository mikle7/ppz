#!/usr/bin/env bash
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create one >/dev/null
ppz_a terminal create two >/dev/null
# --dump-state prints the same JSON the desktop GUI would render. Pipe
# through jq with a stable projection so we don't depend on field ordering.
ppz-desktop --dump-state --ipc="$PPZ_DAEMON_A_SOCK" \
  | jq -c '{logged_in, account_id, handles: [.sources[].handle]}'
