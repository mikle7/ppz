#!/usr/bin/env bash
# Phase 1.5.2: `ppz read <uncollared-pipe>` defaults to the v0.23
# tabular render (HH:MM:SS  <sender>  <body>), matching the existing
# behaviour for inbox/broadcast. Uncollared pipes are the canonical
# messaging primitive — they should render the same way.
#
# Pre-1.5.2 behaviour: IsTabularReadPipe returns true only for "inbox"
# and "broadcast", so uncollared reads fell to the byte-faithful
# default. RED until IsTabularReadPipe (or the case selector in
# runRead) is extended to recognise uncollared targets.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a pipe create room >/dev/null
ppz_a send room "hello" >/dev/null
wait_for 20 "ppz_a reread room --json | jq -r '.payload' | grep -q hello" >/dev/null

# Bare read. Should render the tabular row, not just the payload.
ppz_a read room
