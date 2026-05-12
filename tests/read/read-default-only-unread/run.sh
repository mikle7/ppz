#!/usr/bin/env bash
# Default `ppz read` delivers only what's *new* since the caller's
# session cursor — like `git log` showing only new commits since you
# last looked. Agents polling a pipe shouldn't have to track their own
# watermark or use --since timing heuristics.
#
# Flow:
#   1. Broadcast 3 messages, ppz read drains all 3 (cursor was 0).
#   2. ls now reports unread=0.
#   3. Broadcast 1 more, ppz read drains only the new one.
#   4. ls reports unread=0 again.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create chat >/dev/null
ppz_a send chat.inbox "msg-1" >/dev/null
ppz_a send chat.inbox "msg-2" >/dev/null
ppz_a send chat.inbox "msg-3" >/dev/null
wait_for 20 "ppz_a ls | grep -q msg-3" >/dev/null

echo "--- first read: drains all 3 unread ---"
ppz_a read --bare chat.inbox

echo "--- ls after first read: unread=0 ---"
ppz_a ls | ls_normalize

echo "--- send one more ---"
ppz_a send chat.inbox "msg-4" >/dev/null
wait_for 20 "ppz_a ls | grep -q msg-4" >/dev/null

echo "--- second read: only the new message ---"
ppz_a read --bare chat.inbox

echo "--- ls after second read: unread=0 again ---"
ppz_a ls | ls_normalize
