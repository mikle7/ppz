#!/usr/bin/env bash
# Flood protection: default `ppz read` delivers at most the NEXT 10
# unread (oldest-first, head-N — NOT reread's tail-N), advancing the
# cursor only past what was actually delivered. A truncated read prints
# a "N more unread" trailer so the consumer knows to page; repeated
# invocations walk the backlog in order without skipping anything.
#
# Flow:
#   1. Send 15 messages; first read delivers msg-01..msg-10 + trailer.
#   2. ls reports unread=5 (cursor is honest — only advanced past 10).
#   3. Second read delivers msg-11..msg-15, no trailer.
#   4. ls reports unread=0.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
for i in $(seq -w 1 15); do
  ppz_a send chat.inbox "msg-$i" >/dev/null
done
wait_for 20 "ppz_a ls | grep -q msg-15" >/dev/null

OUT=$(mktemp)
echo "--- first read: next 10 oldest ---"
ppz_a read chat.inbox >"$OUT" 2>/dev/null
grep -oE 'msg-[0-9]+' "$OUT"
echo "--- trailer names the remainder ---"
grep -oE '[0-9]+ more unread' "$OUT"
echo "--- ls after first read: unread=5 ---"
ppz_a ls | ls_normalize

echo "--- second read: remaining 5 ---"
ppz_a read chat.inbox >"$OUT" 2>/dev/null
grep -oE 'msg-[0-9]+' "$OUT"
echo "--- no trailer once drained ---"
grep -c 'more unread' "$OUT" || true
echo "--- ls after second read: unread=0 ---"
ppz_a ls | ls_normalize
