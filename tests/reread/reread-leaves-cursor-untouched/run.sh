#!/usr/bin/env bash
# `ppz reread` is the forensic / replay verb: deliver everything in the
# pipe, never consult or advance the session cursor. Use case: audit a
# pipe's history without affecting another tool's unread count.
#
# Flow:
#   1. Broadcast 3 messages.
#   2. ppz reread delivers all 3.
#   3. ls still reports unread=3 — reread didn't touch the cursor.
#   4. A subsequent default ppz read still drains all 3.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a broadcast -m "msg-1" >/dev/null
ppz_a broadcast -m "msg-2" >/dev/null
ppz_a broadcast -m "msg-3" >/dev/null
wait_for 20 "ppz_a ls | grep -q msg-3" >/dev/null

echo "--- reread: delivers all 3 ---"
ppz_a reread --bare chat.broadcast

echo "--- ls after reread: unread still 3 (cursor not advanced) ---"
ppz_a ls | ls_normalize

echo "--- subsequent default read: still drains all 3 ---"
ppz_a read --bare chat.broadcast

echo "--- ls after default read: unread=0 ---"
ppz_a ls | ls_normalize
