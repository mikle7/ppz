#!/usr/bin/env bash
# `--max-msgs=N` overrides JetStream MaxMsgs. After publishing 8 messages
# to a pipe with cap=5, ls reports buffered=5 (the oldest 3 were
# discarded by the ring buffer).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create chat >/dev/null
ppz_a pipe create ring --max-msgs=5 >/dev/null

for i in 1 2 3 4 5 6 7 8; do
  ppz_a send chat.ring "msg-$i" >/dev/null
done

# Wait for the last message to land.
wait_for 20 "ppz_a reread chat.ring --json | jq -r '.payload' | grep -q msg-8" >/dev/null

# Layout: PIPE UNREAD BUFFERED LAST PAYLOAD. BUFFERED (col $3) is the
# retained-message count. Should be 5, not 8.
ppz_a ls | awk '$1 == "chat.ring" {print "buffered="$3}'
