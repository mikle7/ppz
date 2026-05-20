#!/usr/bin/env bash
# TS-C: a backgrounded `ppz status` inside the wrapped pty resolves to
# cindy. `&` and `nohup` don't break the ppid chain — the background
# bash is still a descendant of the terminal share process. Verified
# empirically in the diagnosis probe.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a unset namespace >/dev/null

PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" \
  ppz terminal share cindy -- sh -c '
    nohup ppz status > /tmp/ts-c-cap.txt 2>&1 &
    BGPID=$!
    wait "$BGPID"
  ' </dev/null >/dev/null 2>&1

grep -E "^current:" /tmp/ts-c-cap.txt
rm -f /tmp/ts-c-cap.txt
