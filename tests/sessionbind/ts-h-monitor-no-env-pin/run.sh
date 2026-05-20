#!/usr/bin/env bash
# TS-H: the point of this spec — the Monitor recipe at
# `internal/cli/agent.go:196` no longer needs the inline
# `PPZ_SESSION=<handle>` pin.
#
# Critical detail (from the review of PR #75): for this to actually
# reproduce the prod failure mode, the in-pty subprocess must be
# spawned via `setsid` — that's what Claude Code's Bash tool does,
# producing a fresh `getsid` value that the daemon has never seen.
# A plain `sh -c` inherits the pty bash's session and would never
# have manifested the bug in the first place.
#
# The acceptance: `ppz ls --watch` invoked inside a setsid'd
# subprocess of cindy's pty resolves to cindy via the daemon's
# ancestor walk, and the watch returns a snapshot with cindy's
# inbox row when a message arrives.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a unset namespace >/dev/null

PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" \
  ppz terminal share cindy -- sh -c '
    sleep 0.5
    # Background a sender that publishes to cindy.inbox after a tick.
    # PPZ_IPC_SOCKET inherits via the wrapped env.
    (sleep 1 && ppz send cindy "hello-monitor") &
    # Recipe under test: setsid (new session leader, fresh getsid)
    # without inline PPZ_SESSION pin. If Layer 1 ancestor-walk works,
    # the daemon binds via the ppid chain (this sh → terminal share)
    # and ls --watch resolves to cindy.
    timeout 5 setsid ppz ls --watch 2>&1 | awk "/^cindy/" | head -1 > /tmp/ts-h-cap.txt
  ' </dev/null >/dev/null 2>&1

if grep -qE "^cindy" /tmp/ts-h-cap.txt 2>/dev/null; then
  echo "monitor-without-env-pin: ok"
else
  echo "monitor-without-env-pin: FAILED (no cindy row in watch output)"
fi
rm -f /tmp/ts-h-cap.txt
