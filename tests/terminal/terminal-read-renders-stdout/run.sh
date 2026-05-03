#!/usr/bin/env bash
# `ppz terminal read <handle>` drains the current contents of <handle>.stdout
# and exits — no follow, no SIGINT needed. Default mode runs the captured
# bytes through a virtual terminal and prints the resulting screen state
# as plain text rows (so TUI cursor moves resolve, no escape pollution).
# `--raw` opts back into byte-faithful output.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal share term -- printf "peek-content" >/dev/null

# Wait for the chunk to land on .stdout.
wait_for 20 "ppz_a ls 2>/dev/null | ls_normalize | grep -E '^term\.stdout 1 ' >/dev/null" >/dev/null

# Default mode: vt10x renders the screen state. Trailing whitespace per
# row is trimmed; trailing all-blank rows are dropped; output ends with
# a single \n on the last non-blank row.
ppz_a terminal read term
