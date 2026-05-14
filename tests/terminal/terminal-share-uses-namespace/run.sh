#!/usr/bin/env bash
# Phase 1.5.2: bare `ppz terminal share` (no positional handle) against
# a current message-kind source at a non-root manifold must add
# stdin/stdout/stdctrl pipes via the daemon's namespace-aware pipe-
# create path. The if-branch in cmdTerminalShare omits Session from
# IPCPipeCreate so the daemon can't stamp the manifold and the server
# 404s when it tries to add pipes to a source at root that doesn't
# exist (the actual source is at <namespace>).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a set namespace pixel >/dev/null

# Message-kind source so source-create sets it as current handle (PTY
# kinds deliberately don't). After this, current_handle=dora at the
# pixel namespace.
ppz_a source create dora >/dev/null

# Bare share (no positional). Uses current source. The wrap writes one
# chunk to dora.stdout via the auto-provisioned pipe.
ppz_a terminal share -- printf "wrapped-output" >/dev/null

# Verify the bytes landed at pixel.dora.stdout (the only place they
# could land if Session was threaded correctly).
wait_for 20 "ppz_a reread dora.stdout --json | jq -r '.payload' | tr -d '\r' | grep -q wrapped-output" >/dev/null
ppz_a reread dora.stdout --json | jq -r '.payload' | tr -d '\r' | grep -o wrapped-output | head -1
