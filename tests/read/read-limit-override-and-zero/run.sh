#!/usr/bin/env bash
# `ppz read -l N` overrides the default cap of 10 with head-N (the NEXT
# N oldest unread — unlike `reread -l`, which is tail-N over history).
# `-l 0` opts out of the cap entirely and drains everything unread.
#
# Run under --bare to pin that script-stable output modes stay clean:
# no trailer line even when the read is truncated (--bare promises
# payload-only output; same suppression contract as --raw/--json).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
for i in 1 2 3 4 5; do
  ppz_a send chat.inbox "msg-$i" >/dev/null
done
wait_for 20 "ppz_a ls | grep -q msg-5" >/dev/null

echo "--- read -l 2: next 2 oldest, no trailer under --bare ---"
ppz_a read --bare chat.inbox -l 2

echo "--- read -l 0: drains all remaining ---"
ppz_a read --bare chat.inbox -l 0

echo "--- ls: unread=0 ---"
ppz_a ls | ls_normalize
