#!/usr/bin/env bash
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create foo >/dev/null
echo "hello from stdin" | ppz_a broadcast
# After broadcast, ls must show the payload (eventually consistent — wait for
# the server-side subscriber to write last_broadcast_payload).
wait_for 20 "ppz_a ls | grep -q 'hello from stdin'" >/dev/null
ppz_a ls | ls_normalize
