#!/usr/bin/env bash
# Phase 1.5.2: `ppz terminal read H` reads H.stdout. When H is at a
# non-root namespace, the daemon must resolve to <manifold>.H.stdout
# (via the HandleManifold cache populated from /api/v1/sources). Pre-
# 1.5.2 untested — locks in the resolution behaviour.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a set namespace pixel >/dev/null
ppz_a terminal create dora >/dev/null

# Publish some bytes to the pty source via terminal share so stdout has
# content to read.
ppz_a terminal share dora -- printf "stdout-bytes" >/dev/null
wait_for 20 "ppz_a reread pixel.dora.stdout --json | jq -r '.payload' | tr -d '\r' | grep -q stdout-bytes" >/dev/null

# Bare `terminal read dora` — no manifold in the command. Daemon should
# find pixel.dora.stdout via the handle→manifold cache.
ppz_a terminal read dora --raw 2>/dev/null | tr -d '\r' | grep -o stdout-bytes | head -1
