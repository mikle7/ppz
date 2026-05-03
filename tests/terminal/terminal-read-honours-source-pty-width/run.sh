#!/usr/bin/env bash
# `ppz terminal read <h>` (and `ppz read <h>.stdout --tty`) must render
# at the source pty's actual dimensions, NOT the hardcoded 200×60 grid.
# The latest <h>.stdctrl resize message carries those dimensions; the
# daemon should look it up and pass cols/rows through to the renderer
# via a leading meta event in the read stream.
#
# Repro shape: simulate a 220-col source by publishing a resize event
# directly to stdctrl, then push a 210-X line to stdout. The renderer
# must show all 210 X's on a single line (no col-200 wrap from vt10x).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
# `terminal share` provisions the pty source + auto-creates stdctrl.
# `true` exits immediately; we'll publish to the pipes directly.
ppz_a terminal share wide -- true >/dev/null
wait_for 20 "ppz_a ls | grep -q '^wide.stdctrl'" >/dev/null

# Simulate a 220×50 source by writing a resize event to stdctrl.
ppz_a send wide.stdctrl '{"type":"resize","cols":220,"rows":50}' >/dev/null
wait_for 20 "ppz_a reread wide.stdctrl --json | tail -1 | jq -r '.payload' | grep -q '\"cols\":220'" >/dev/null

# Push a 210-X line to stdout — exceeds the 200-col default but fits
# in the 220-col actual source size.
LONG=$(printf 'X%.0s' $(seq 1 210))
ppz_a send wide.stdout "$LONG" >/dev/null
wait_for 20 "ppz_a reread wide.stdout --raw | tr -d '\\n' | grep -q 'X\\{210\\}'" >/dev/null

echo "--- terminal read renders all 210 X's on a single row ---"
# At correct 220 cols: longest run of contiguous X's on one line = 210.
# At broken default 200 cols: vt10x wraps so longest run = 200.
ppz_a terminal read wide \
  | awk '/^X+$/{n=length($0); if(n>max)max=n} END{print "longest_x_run="max+0}'
