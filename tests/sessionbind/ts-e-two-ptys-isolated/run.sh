#!/usr/bin/env bash
# TS-E: two concurrent ptys (cindy + bob) don't cross-contaminate.
# Each pty's in-pty `ppz status` resolves to its OWN handle. Tests
# that ancestor-match precedence isolates correctly when multiple
# bindings are registered.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a unset namespace >/dev/null

# Spawn cindy's pty and bob's pty concurrently. Each writes its status
# into a separate temp file so we can attribute correctly.
(
  PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" \
    ppz terminal share cindy -- sh -c '
      ppz status 2>&1 | grep -E "^(current source|namespace):" > /tmp/ts-e-cindy.txt
    ' </dev/null >/dev/null 2>&1
) &
PID_C=$!

(
  PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" \
    ppz terminal share bob -- sh -c '
      ppz status 2>&1 | grep -E "^(current source|namespace):" > /tmp/ts-e-bob.txt
    ' </dev/null >/dev/null 2>&1
) &
PID_B=$!

wait "$PID_C"
wait "$PID_B"

echo "cindy_pty: $(cat /tmp/ts-e-cindy.txt 2>/dev/null)"
echo "bob_pty:   $(cat /tmp/ts-e-bob.txt 2>/dev/null)"
rm -f /tmp/ts-e-cindy.txt /tmp/ts-e-bob.txt
