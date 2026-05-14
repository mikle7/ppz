#!/usr/bin/env bash
# Phase 1.5.1: `ppz reread LEAF` (bare) reads from the uncollared
# pipe LEAF at current namespace. Current handle does NOT affect read
# routing (same rule as create/destroy).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a pipe create room >/dev/null
ppz_a send room "uncollared payload" >/dev/null 2>&1
# Set a different handle to confirm read still goes to uncollared.
ppz_a source create alice >/dev/null
ppz_a reread room -l 1
