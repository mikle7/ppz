#!/usr/bin/env bash
# TS-I: `ppz set handle bob` inside cindy's pty overrides the binding's
# default (cindy). Subsequent `ppz status` in the same pty reports bob.
# `ppz unset handle` clears the override; next status re-fires
# auto-write and reports cindy again.
#
# Tests that State.Current(SessionKey) wins over binding.BoundHandle
# (RS-11) and that auto-write re-populates on a cleared current (AC-6).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a unset namespace >/dev/null

# Create a bob source ahead of time so `set handle bob` works.
ppz_a source create bob >/dev/null
ppz_a unset handle >/dev/null

PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" \
  ppz terminal share cindy -- sh -c '
    # Step 1: baseline — auto-write populates cindy.
    env -u PPZ_CURRENT_HANDLE -u PPZ_SESSION ppz status 2>&1 | grep -E "^current source:" | sed "s/^/step1 /" > /tmp/ts-i-cap.txt
    # Step 2: explicit override to bob.
    ppz set handle bob >/dev/null
    env -u PPZ_CURRENT_HANDLE -u PPZ_SESSION ppz status 2>&1 | grep -E "^current source:" | sed "s/^/step2 /" >> /tmp/ts-i-cap.txt
    # Step 3: unset → next status re-fires auto-write back to cindy.
    ppz unset handle >/dev/null
    env -u PPZ_CURRENT_HANDLE -u PPZ_SESSION ppz status 2>&1 | grep -E "^current source:" | sed "s/^/step3 /" >> /tmp/ts-i-cap.txt
  ' </dev/null >/dev/null 2>&1

cat /tmp/ts-i-cap.txt
rm -f /tmp/ts-i-cap.txt
