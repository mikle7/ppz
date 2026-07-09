#!/usr/bin/env bash
# --limit is the long form of -l on `ppz reread`: same tail-N replay
# semantics (the N MOST RECENT retained messages, printed oldest-first),
# cursor untouched. Both spellings must behave identically.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
for i in 1 2 3 4 5; do
  ppz_a send chat.inbox "msg-$i" >/dev/null
done
wait_for 20 "ppz_a ls | grep -q msg-5" >/dev/null

ppz_a reread --bare chat.inbox --limit 2
