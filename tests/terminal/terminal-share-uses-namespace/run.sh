#!/usr/bin/env bash
# Phase 1.5.2: `ppz terminal share <handle>` against an EXISTING pty
# source at a non-root manifold should wrap correctly. Distinct from
# the auto-provisioning code path (covered by
# agent/agent-create-uses-current-namespace) — this exercises the
# if-branch in cmdTerminalShare that creates missing stdin/stdout
# pipes for a pre-existing source.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a set namespace pixel >/dev/null

# Pre-create the source at pixel via the explicit `terminal create`
# path, then share against it.
ppz_a terminal create dora >/dev/null
ppz_a terminal share dora -- printf "wrapped-output" >/dev/null

# Wait for the wrapped output to land on the stdout pipe at
# pixel.dora.stdout.
wait_for 20 "ppz_a reread pixel.dora.stdout --json | jq -r '.payload' | tr -d '\r' | grep -q wrapped-output" >/dev/null
ppz_a reread pixel.dora.stdout --json | jq -r '.payload' | tr -d '\r' | grep -o wrapped-output | head -1
