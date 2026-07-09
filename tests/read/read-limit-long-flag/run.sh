#!/usr/bin/env bash
# --limit is the long form of -l on `ppz read`: same head-N semantics
# (the NEXT N oldest unread), same -l 0 opt-out, same --tail mutual
# exclusion. Both spellings must behave identically.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
for i in 1 2 3 4 5; do
  ppz_a send chat.inbox "msg-$i" >/dev/null
done
wait_for 20 "ppz_a ls | grep -q msg-5" >/dev/null

echo "--- read --limit 2: next 2 oldest ---"
ppz_a read --bare chat.inbox --limit 2

echo "--- read --limit 0: drains all remaining ---"
ppz_a read --bare chat.inbox --limit 0

echo "--- read --limit with --tail: rejected ---"
ppz_a read chat.inbox --limit 5 --tail >/dev/null 2>&1
echo "rc=$?"

echo "--- ls: unread=0 ---"
ppz_a ls | ls_normalize
