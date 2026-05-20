#!/usr/bin/env bash
# TS-B: a `ppz status` issued two subprocess hops deep inside the
# wrapped pty still resolves to cindy. Validates the ancestor walk
# covers more than depth 1.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a unset namespace >/dev/null

# Outer sh → inner bash → ppz. Inner bash gets a fresh session of its
# own (subprocess managers tend to `setsid`); the ancestor pid chain
# still leads back through outer sh → terminal share → daemon binding.
PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" \
  ppz terminal share cindy -- sh -c '
    bash -c "env -u PPZ_CURRENT_HANDLE -u PPZ_SESSION ppz status 2>&1 | grep -E \"^current source:\" > /tmp/ts-b-cap.txt"
  ' </dev/null >/dev/null 2>&1

cat /tmp/ts-b-cap.txt
rm -f /tmp/ts-b-cap.txt
