#!/usr/bin/env bash
# TS-D: a grandchild process (sh → sh → ppz) resolves to cindy via the
# ancestor pid chain. Covers the agent-style "sub-shell, sub-agent"
# spawn pattern observed in the Claude Code Agent tool probe — multiple
# subprocess layers between caller and the registered share pid.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a unset namespace >/dev/null

PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" \
  ppz terminal share cindy -- sh -c '
    sh -c "
      sh -c \"ppz status 2>&1 | grep -E \\\"^current:\\\" > /tmp/ts-d-cap.txt\"
    "
  ' </dev/null >/dev/null 2>&1

cat /tmp/ts-d-cap.txt
rm -f /tmp/ts-d-cap.txt
