#!/usr/bin/env bash
# SI-4: namespace set in cindy's pty propagates to subprocesses in
# the same pty. `ppz set namespace team-a` writes against the bound
# session key; later `ppz status` (or `ppz get namespace`) in the
# same pty sees `team-a`.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a unset namespace >/dev/null

PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" \
  ppz terminal share cindy -- sh -c '
    # First subprocess: set namespace.
    env -u PPZ_CURRENT_HANDLE -u PPZ_SESSION ppz set namespace team-a >/dev/null
    # Second subprocess: read it back. Strip env so we test daemon-side
    # binding resolution (not env-pin).
    env -u PPZ_CURRENT_HANDLE -u PPZ_SESSION ppz status 2>&1 | grep -E "^namespace:" > /tmp/si-4-cap.txt
  ' </dev/null >/dev/null 2>&1

cat /tmp/si-4-cap.txt
rm -f /tmp/si-4-cap.txt
