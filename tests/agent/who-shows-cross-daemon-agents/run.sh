#!/usr/bin/env bash
# ppz who should list agents from all daemons in the same org, not only
# agents whose heartbeats the local daemon received via its own
# handleSend path.
#
# Both daemons log in as alpha. agent-a is started on daemon-a at the
# root manifold; agent-b is started on daemon-b inside namespace "ns".
# The namespaced agent exercises the bare-handle invariant: handleSend
# on daemon-b stamps its own cache with the bare handle "agent-b"
# (req.Handle), so the subscriber on daemon-a must also stamp under
# "agent-b" — not "ns.agent-b" — for ppz who to render the same row
# shape on both daemons and avoid duplicating the agent twice on the
# publisher (NATS echoes a daemon's own publishes back to its own
# subscriptions).
. /tests/lib/common.sh

cleanup() {
  kill "$PID_A" "$PID_B" 2>/dev/null || true
  wait "$PID_A" "$PID_B" 2>/dev/null || true
}
trap cleanup EXIT

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_b daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

# daemon-b puts agent-b at namespace "ns" so the wire subject becomes
# <acct>.ns.agent-b.heartbeat. handleSend on daemon-b still stamps
# the cache under bare "agent-b"; the subscriber on daemon-a must do
# the same.
ppz_b set namespace ns >/dev/null

# Start one agent per daemon. runHeartbeat fires its first beat
# immediately on startup, so a heartbeat lands in each daemon's cache
# even though sleep 10 keeps the PTY alive long enough to assert.
ppz_a terminal share agent-a -- sleep 10 </dev/null &
PID_A=$!
ppz_b terminal share agent-b -- sleep 10 </dev/null &
PID_B=$!

# Gate on each daemon seeing its own agent — proves the heartbeat was
# published via NATS before we query daemon-a.
wait_for 50 "ppz_b who | grep -q agent-b" || { echo "timeout: agent-b never appeared in daemon-b"; exit 1; }
wait_for 50 "ppz_a who | grep -q agent-a" || { echo "timeout: agent-a never appeared in daemon-a"; exit 1; }

# Gate on cross-daemon propagation explicitly so a slow NATS round-trip
# produces a clear timeout instead of a diff failure on the assertion
# below.
wait_for 50 "ppz_a who | grep -q agent-b" || { echo "timeout: agent-b never propagated to daemon-a"; exit 1; }

# daemon-a's ppz who must list both agents under bare handles. Filter
# to ^agent- so heartbeats leaked from prior scenarios (e.g. "cindy"
# from agent-create-uses-current-namespace) don't pollute the
# assertion — the in-memory cache persists across scenarios. A
# manifold-prefixed key ("ns.agent-b") would not match ^agent- and
# would fail the diff.
ppz_a who | awk '/^agent-/ {print $1}' | sort
