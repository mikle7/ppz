#!/usr/bin/env bash
# SI-1: each pty sees its own current handle. Two concurrent ptys
# (cindy + bob); inside each, `ppz get handle` returns the binding's
# handle. No cross-pollution.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a unset namespace >/dev/null

(
  PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" \
    ppz terminal share cindy -- sh -c '
      ppz get handle 2>/dev/null > /tmp/si-1-cindy.txt
    ' </dev/null >/dev/null 2>&1
) &
PC=$!
(
  PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" \
    ppz terminal share bob -- sh -c '
      ppz get handle 2>/dev/null > /tmp/si-1-bob.txt
    ' </dev/null >/dev/null 2>&1
) &
PB=$!

wait "$PC"
wait "$PB"

echo "cindy_pty: $(cat /tmp/si-1-cindy.txt 2>/dev/null)"
echo "bob_pty:   $(cat /tmp/si-1-bob.txt 2>/dev/null)"
rm -f /tmp/si-1-cindy.txt /tmp/si-1-bob.txt
