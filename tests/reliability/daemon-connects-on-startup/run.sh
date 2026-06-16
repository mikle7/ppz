#!/usr/bin/env bash
# RED — cold-start connect gap reported after the v0.48.1 upgrade.
#
# A freshly-(re)started daemon that is already logged in does NOT bring
# its NATS connection up on its own: Run() has no proactive connect, so
# the first connection is established lazily by whatever IPC command
# first calls ensureNATS. Until then `ppz status` shows `nats: unknown`
# and the daemon receives no pushed messages (heartbeats, inbox). The
# user saw exactly this after `ppz upgrade` restarted the daemon: two
# `ppz status` calls showed `nats: unknown`, and only `ppz ls` brought
# the connection up.
#
# Contract: after the daemon process restarts while logged in, the
# connection comes up on its own — observable via `ppz status` WITHOUT
# any connection-triggering command in between.
#
# Probe discipline: we poll `ppz status` ONLY. Unlike `ls`/`send`,
# status never routes through ensureNATS (it does an HTTP login-check,
# not a NATS call — see internal/daemon/ipc.go ipcStatus), so it cannot
# itself establish the connection and mask the gap. Using `ls` here
# would make the test pass even on the buggy build.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

# Prove the happy path while up — this also establishes a connection on
# the current process and doubles as a setup guard. If `ls` doesn't
# work here, the post-restart assertion would be meaningless.
if ppz_a ls >/dev/null 2>&1; then echo "setup=OK"; else echo "setup=BROKEN"; fi

# Restart the daemon PROCESS (what `ppz upgrade` / a reboot does). The
# fresh process reads credentials from the persisted home volume but
# must bring NATS up by itself.
docker restart compose-daemon-a-1 >/dev/null

# Wait for the new daemon to accept IPC again. `ppz status` succeeding
# (rc 0) only needs the socket — not a NATS connection — so this loop
# does not itself trigger a connect.
wait_for 300 'ppz_a status >/dev/null 2>&1'

# Now poll status (NEVER ls/send) for a self-established connection.
if wait_for 300 'ppz_a status | grep -q "^nats: connected"'; then
  echo "verdict=CONNECTED-ON-STARTUP"
else
  echo "verdict=STUCK-UNKNOWN"
fi
