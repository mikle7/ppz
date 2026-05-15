#!/usr/bin/env bash
# Phase 1.5.2: `ppz agent create H` lands the source at the session's
# current namespace, mirroring `ppz source create` and `ppz terminal
# create`. v0.31.1 regression: cmdAgentCreate → cmdTerminalShare omits
# Session from IPCCreate so the daemon can't find the session's
# namespace and falls back to root.
#
# Test uses `--claude --new-window` is heavy + interactive. We exercise
# the same code path (cmdTerminalShare auto-provisioning a new pty
# source) via `terminal share <handle> -- printf hi` which is what
# agent create runs internally minus the harness binary.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a set namespace pixel >/dev/null

# Auto-provision a new pty-kind source via the same path cmdAgentCreate
# uses. Should land at pixel.cindy, not root cindy.
ppz_a terminal share cindy -- printf "hi" >/dev/null
wait_for 20 "ppz_a ls | grep -q '^pixel\.cindy\.'" >/dev/null

# Look at the pipe rows; expect all to be pixel-namespaced.
ppz_a ls | awk '$1 ~ /cindy\./ {print $1}' | sort
