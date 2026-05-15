#!/usr/bin/env bash
# Phase 1.5.2: `ppz read LEAF` (bare, no dot) reads the uncollared pipe
# LEAF at the session's current namespace. Mirrors `ppz pipe create
# LEAF` + `ppz send LEAF` which both already stamp manifold from the
# session. Pre-1.5.2 untested — locks in the read resolution.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a set namespace pixel >/dev/null

# Create + send to an uncollared pipe at pixel.
ppz_a pipe create room >/dev/null
ppz_a send room "from-namespace" >/dev/null
wait_for 20 "ppz_a reread room --json | jq -r '.payload' | grep -q from-namespace" >/dev/null

# Bare read; daemon should resolve to pixel.room (the uncollared pipe
# at the session's current namespace) rather than a root `room`.
ppz_a read room --bare 2>/dev/null | head -1
