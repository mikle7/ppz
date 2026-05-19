#!/usr/bin/env bash
# RED: ppz who should list agents from all daemons in the same org,
# not only agents whose heartbeats the local daemon received via its
# own handleSend path.
#
# Both daemons log in as alpha. One agent is started on each. daemon-b's
# agent heartbeat is only stamped in daemon-b's local cache — daemon-a
# never sees it via handleSend. ppz_a who should still list it.
. /tests/lib/common.sh

cleanup() {
  kill "$PID_A" "$PID_B" 2>/dev/null || true
  wait "$PID_A" "$PID_B" 2>/dev/null || true
}
trap cleanup EXIT

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_b daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

# Start one agent per daemon. runHeartbeat fires its first beat
# immediately on startup, so a heartbeat lands in each daemon's cache
# even though sleep 10 keeps the PTY alive long enough to assert.
ppz_a terminal share agent-a -- sleep 10 </dev/null &
PID_A=$!
ppz_b terminal share agent-b -- sleep 10 </dev/null &
PID_B=$!

# Gate on daemon-b seeing agent-b — proves the heartbeat was published
# via NATS before we query daemon-a.
wait_for 50 "ppz_b who | grep -q agent-b" || { echo "timeout: agent-b never appeared in daemon-b"; exit 1; }
wait_for 50 "ppz_a who | grep -q agent-a" || { echo "timeout: agent-a never appeared in daemon-a"; exit 1; }

# daemon-a's ppz who must list both agents. Filter to ^agent- so
# heartbeats leaked from prior scenarios (e.g. "cindy" from
# agent-create-uses-current-namespace) don't pollute the assertion —
# the in-memory cache persists across scenarios. Without the fix only
# agent-a appears because handleWho reads only the local daemon's cache.
ppz_a who | awk '/^agent-/ {print $1}' | sort
