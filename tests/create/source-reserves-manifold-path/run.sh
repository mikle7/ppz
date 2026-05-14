#!/usr/bin/env bash
# Phase 1.5.1: a source `X` at manifold M reserves the path `M.X` as a
# manifold-prefix. Otherwise wire-level collisions occur (e.g. source
# `team1` at root auto-provisions `team1.inbox`; an uncollared pipe
# `inbox` at manifold `team1` would publish to the same NATS subject).
#
# Asserts: source `team1` at root + attempt to create uncollared pipe
# under manifold `team1` (any name) is rejected.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a source create team1 >/dev/null
ppz_a unset handle >/dev/null
ppz_a set namespace team1 >/dev/null
ppz_a pipe create chat
echo "exit2=$?"
