#!/usr/bin/env bash
# Bug repro: once all messages have been purged from a pipe's buffer
# (here via short TTL; same shape applies to --max-msgs ring-buffer
# eviction), `ppz ls` reports stale UNREAD counts and `ppz read`
# returns nothing yet the cursor never advances — so UNREAD stays
# "stuck" until a new message arrives. The user's workaround was to
# send a fresh message just to bump the cursor.
#
# Fix: `ls` must cap UNREAD at BUFFERED (there's no point reporting
# unread messages the user can never actually read), so once the
# buffer drains UNREAD reads zero — both before and after `read`.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a pipe create chat.bug --ttl=2s >/dev/null

for i in 1 2 3 4; do
  ppz_a send chat.bug "msg-$i" >/dev/null
done

# Wait until all four sends have landed in the stream, then for the
# JetStream MaxAge reaper to drain the buffer to zero. ls_normalize
# fuses NAMESPACE+PIPE so $1=chat.bug, $2=UNREAD, $3=BUFFERED.
wait_for 30  'ppz_a ls | ls_normalize | awk "\$1 == \"chat.bug\" { exit (\$3 == 4 ? 0 : 1) }"' >/dev/null
wait_for 150 'ppz_a ls | ls_normalize | awk "\$1 == \"chat.bug\" { exit (\$3 == 0 ? 0 : 1) }"' >/dev/null

echo "--- after purge, before read ---"
ppz_a ls | ls_normalize | awk '$1 == "chat.bug" {print "unread="$2, "buffered="$3}'

ppz_a read chat.bug >/dev/null

echo "--- after read ---"
ppz_a ls | ls_normalize | awk '$1 == "chat.bug" {print "unread="$2, "buffered="$3}'
