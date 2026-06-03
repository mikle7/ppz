#!/usr/bin/env bash
# `terminal share H` auto-subscribes H.inbox under "H" (idempotent against
# the source-create path). Visible in H's own session's subs ls.
. /tests/lib/common.sh
cleanup() { kill "$PID" 2>/dev/null || true; wait "$PID" 2>/dev/null || true; }
trap cleanup EXIT
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal share agentx -- sleep 30 </dev/null &
PID=$!
wait_for 100 "ppz_a who --json | grep -q '\"handle\": *\"agentx\"'" \
  || { echo "agentx never came online"; exit 1; }
PPZ_SESSION=agentx ppz_a subs ls | ls_normalize | awk '{print $1}'
