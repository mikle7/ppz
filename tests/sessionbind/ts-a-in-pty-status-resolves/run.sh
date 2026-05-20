#!/usr/bin/env bash
# TS-A (smoking-gun): a `ppz status` issued inside a wrapped pty
# resolves to the pty's owning handle, without any PPZ_SESSION env
# propagation. The bug today: the subprocess sees a fresh `getsid`
# value the daemon has never seen → returns `no current source`.
# After Layer 1+2: the daemon walks the caller's ancestor pid chain,
# finds the `ppz terminal share` pid registered for cindy, returns
# current handle = cindy.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a unset namespace >/dev/null

# Wrap a one-shot `ppz status` inside cindy's pty. PPZ_IPC_SOCKET is
# inherited via the explicit assignment-before-command on the outer
# share invocation, so the wrapped status hits the same daemon.
# Output redirected to a file so we don't have to drain cindy.stdout.
PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" \
  ppz terminal share cindy -- sh -c '
    ppz status 2>&1 | grep -E "^(current|namespace):" > /tmp/ts-a-cap.txt
  ' </dev/null >/dev/null 2>&1

# Show what the in-pty `ppz status` reported.
cat /tmp/ts-a-cap.txt
rm -f /tmp/ts-a-cap.txt
