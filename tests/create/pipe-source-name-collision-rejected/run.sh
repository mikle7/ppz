#!/usr/bin/env bash
# Phase 1.5.1 first-wins collision rule, reverse direction: uncollared
# pipe `foo` exists at manifold M → can't create source `foo` at same
# manifold.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a pipe create foo >/dev/null
ppz_a source create foo 2>&1
echo "exit2=$?"
