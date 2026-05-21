#!/usr/bin/env bash
# TS-H: the point of this spec — the Monitor recipe at
# `internal/cli/agent.go:196` no longer needs the inline
# `PPZ_SESSION=<handle>` pin.
#
# We mimic the recipe shape without the env pin: a subprocess inside
# cindy's wrapped pty runs `ppz ls --watch` with PPZ_SESSION +
# PPZ_CURRENT_HANDLE explicitly stripped. With Layer 1's ancestor walk,
# the daemon binds the subprocess to cindy via its ppid chain (sh →
# terminal share, whose pid is the registered binding) and the watch
# resolves to cindy's pipes.
#
# Note: this fixture does NOT use `setsid`. Without `-f`, `setsid`
# reparents the program to init (PPid=1), breaking the ppid chain —
# that's a documented Layer 1 limitation, not the case the Monitor
# recipe actually exercises. The recipe is a plain background loop in
# the wrapped pty, which is what we test here.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a unset namespace >/dev/null

PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" \
  ppz terminal share cindy -- sh -c '
    sleep 0.5
    # Background a sender that publishes to cindy.inbox after a tick.
    # Inside the pty, PPZ_CURRENT_HANDLE=cindy is set in env, so this
    # send works without --from. The MONITOR side (below) tests Layer 1.
    (sleep 1 && ppz send cindy "hello-monitor") &
    # Recipe under test: ls --watch with env stripped to PROVE the
    # binding-via-ancestor-walk path. Without env, sessionID() falls
    # back to getsid-based; daemon resolver matches binding via ppid.
    # PR #73 added a NAMESPACE column to `ls` output, so handle.pipe is
    # no longer at column 0. Match anywhere in the line.
    timeout 5 env -u PPZ_CURRENT_HANDLE -u PPZ_SESSION ppz ls --watch 2>&1 | awk "/cindy\.inbox/" | head -1 > /tmp/ts-h-cap.txt
  ' </dev/null >/dev/null 2>&1

if grep -q 'cindy\.inbox' /tmp/ts-h-cap.txt 2>/dev/null; then
  echo "monitor-without-env-pin: ok"
else
  echo "monitor-without-env-pin: FAILED (no cindy row in watch output)"
fi
rm -f /tmp/ts-h-cap.txt
