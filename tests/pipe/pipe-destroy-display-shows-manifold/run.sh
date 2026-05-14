#!/usr/bin/env bash
# Phase 1.5.1: `destroyed pipe=...` line shows the resolved path
# including manifold for uncollared destroys at non-root namespace.
# Pre-1.5.1 the reply carried Manifold but PrintPipeDestroy ignored
# it, rendering just `destroyed pipe=room` even when the destroyed
# pipe was actually at manifold `red-team`.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a set namespace red-team >/dev/null
ppz_a pipe create room >/dev/null
ppz_a pipe destroy room
