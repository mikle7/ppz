#!/usr/bin/env bash
# Lock in the "pipe page renders every buffered message, in order" claim.
# Publishes 50 numbered messages, then asserts the rendered page contains
# exactly 50 data-message rows with the first being msg-001 and the last
# msg-050. Defends against:
#   - accidentally truncating the loop (e.g. only rendering the latest 10)
#   - reversing the iteration order
#   - off-by-one on FirstSeq/LastSeq
. /tests/lib/common.sh
auth_as_foo

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create chat >/dev/null

for i in $(seq 1 50); do
  ppz_a send chat.inbox "msg-$(printf '%03d' "$i")" >/dev/null
done

# Wait until the last send has propagated to the sources table — that
# means the JetStream stream has it too, which is what the page reads.
wait_for 20 "ppz_a ls | grep -q msg-050" >/dev/null

# data-message format is "<id>:<rfc3339>:<payload>". Extract the payload (3rd
# colon-delimited field).
curl_server "/orgs/alpha/sources/chat/pipes/inbox" \
  | grep -oE 'data-message="[^"]+"' \
  | sed -E 's/.*Z:([^"]+)".*/\1/' > /tmp/payloads

echo "count=$(wc -l < /tmp/payloads | tr -d ' ')"
echo "first=$(head -1 /tmp/payloads)"
echo "last=$(tail -1 /tmp/payloads)"
