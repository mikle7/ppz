#!/usr/bin/env bash
# Phase 1.5.2: `ppz terminal read H` reads H.stdout. When H is at a
# non-root namespace, the daemon must resolve to <manifold>.H.stdout
# (via the HandleManifold cache populated from /api/v1/sources). Pre-
# 1.5.2 untested — locks in the resolution behaviour.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a set namespace pixel >/dev/null

# Auto-provision pixel.dora via terminal share + wrap a printf so
# stdout has content. terminal share does both source-create and pipe-
# attachment in one go (cmdTerminalShare else-branch).
ppz_a terminal share dora -- printf "stdout-bytes" >/dev/null
ppz_a ls >/dev/null  # populate HandleManifold cache before the next read

# Bare `terminal read dora` — no manifold in the command. Daemon should
# find pixel.dora.stdout via the handle→manifold cache.
ppz_a terminal read dora --raw 2>/dev/null | tr -d '\r' | grep -o stdout-bytes | head -1
