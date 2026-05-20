#!/usr/bin/env bash
# TS-H: the point of this spec — the Monitor recipe at
# `internal/cli/agent.go:196` no longer needs the inline
# `PPZ_SESSION=<handle>` pin. We simulate a Monitor-shaped loop
# inside the pty without that pin and verify the in-pty `ppz ls
# --watch` resolves to cindy via ancestor walk.
#
# After this spec lands, the prompt should be updated accordingly —
# that change is tracked in task #9.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a unset namespace >/dev/null

# Send a message to cindy.inbox so `ls --watch` has something to wake
# on. Then run a single iteration of the Monitor-style loop without
# the inline PPZ_SESSION pin.
PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" \
  ppz terminal share cindy -- sh -c '
    # Wait briefly for the share to be fully registered + bound.
    sleep 0.5
    # Background a sender that publishes to cindy.inbox after a tick.
    (sleep 1 && ppz send cindy "hello-monitor") &
    # The recipe: no PPZ_SESSION= pin. Watch waits for unread, then
    # prints a snapshot and exits. With ancestor binding working,
    # the snapshot should include cindys inbox row with unread > 0.
    timeout 5 ppz ls --watch 2>&1 | awk "/^cindy/" | head -1 > /tmp/ts-h-cap.txt
  ' </dev/null >/dev/null 2>&1

# Acceptance: the watch returned a cindy row (proving the in-pty
# command resolved cindys session correctly without env pin).
if grep -qE "^cindy" /tmp/ts-h-cap.txt 2>/dev/null; then
  echo "monitor-without-env-pin: ok"
else
  echo "monitor-without-env-pin: FAILED (no cindy row in watch output)"
fi
rm -f /tmp/ts-h-cap.txt
