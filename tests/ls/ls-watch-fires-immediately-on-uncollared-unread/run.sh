#!/usr/bin/env bash
# Level-triggered watch must fire immediately when unread sits on an
# UNCOLLARED pipe — not just on collared (source-owned) pipes. Pre-fix
# `hasUnread` walks reply.Sources only, so an org whose only unread is
# on an uncollared pipe hangs until the 30s test cap.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1
ppz_a pipe create plaza >/dev/null
ppz_a send plaza "preexisting" >/dev/null
wait_for 20 "ppz_a ls 2>/dev/null | grep -q preexisting" >/dev/null

# Watch should return immediately (level-triggered) because plaza has unread.
echo "--- watch returns immediately (uncollared level-triggered) ---"
ppz_a ls --watch | ls_normalize
