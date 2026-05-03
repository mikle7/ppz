#!/usr/bin/env bash
# Regression: when we wrap a child in a PTY, the line discipline must
# keep cooked-mode output processing (OPOST) ON so each \n the child
# emits becomes \r\n on the master read side. Without this, `ls -1`
# (and every other newline-printing program) renders as a staircase
# in the source terminal because the local emulator gets \n alone —
# advance line but don't return to column 0.
#
# Probe: a child writes "a\nb\n" — 4 bytes pre-OPOST, 6 bytes post-OPOST
# (\n → \r\n twice). The .stdout stream captures master read bytes
# verbatim, so summing payload lengths tells us which mode the PTY is
# in.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal share opost-test -- printf "a\nb\n" >/dev/null

wait_for 20 "ppz_a reread opost-test.stdout --json | head -1 | grep -q payload" >/dev/null

bytes=$(ppz_a reread opost-test.stdout --json | jq -r '.payload | length' | awk '{s+=$1} END {print s+0}')
echo "stdout-bytes=$bytes"

# 6 = OPOST on (\n → \r\n applied). 4 = OPOST off (raw \n only).
if [ "$bytes" -eq 6 ]; then
  echo "opost-on=yes"
else
  echo "opost-on=no"
fi
