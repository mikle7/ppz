#!/usr/bin/env bash
# SI-5 (negative): namespaces don't leak across ptys. Cindy sets
# namespace team-a; bob's separate pty should see no namespace set.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a unset namespace >/dev/null

PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" \
  ppz terminal share cindy -- sh -c '
    ppz set namespace team-a >/dev/null
    ppz status 2>&1 | grep -E "^(current source|namespace):" > /tmp/si-5-cindy.txt
  ' </dev/null >/dev/null 2>&1
echo "cindy_pty: $(cat /tmp/si-5-cindy.txt 2>/dev/null)"

# Bob's pty separately — no namespace set in his session.
PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" \
  ppz terminal share bob -- sh -c '
    if ppz status 2>&1 | grep -qE "^(current source|namespace):"; then
      ppz status 2>&1 | grep -E "^(current source|namespace):" > /tmp/si-5-bob.txt
    else
      echo "namespace: (none)" > /tmp/si-5-bob.txt
    fi
  ' </dev/null >/dev/null 2>&1
echo "bob_pty:   $(cat /tmp/si-5-bob.txt 2>/dev/null)"

rm -f /tmp/si-5-cindy.txt /tmp/si-5-bob.txt
