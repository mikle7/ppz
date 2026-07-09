#!/usr/bin/env bash
# --limit must work on `ppz terminal read` too. Its argv splitter keeps
# its OWN value-flag map (cmdTerminalRead, terminal.go) — separate from
# splitReadArgs — to step over value tokens when finding the positional
# handle. If that map misses the long form, the value token is misparsed
# as a second positional handle and the command usage-exits. Pins
# -l / --limit equivalence on this surface: tail-N of <handle>.stdout,
# rendered through vt10x, in either flag order.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal share term -- printf "one" >/dev/null
wait_for 20 "ppz_a ls 2>/dev/null | ls_normalize | grep -E '^term\.stdout 1 ' >/dev/null" >/dev/null
ppz_a send term.stdout "two" >/dev/null
ppz_a send term.stdout "three" >/dev/null
wait_for 20 "ppz_a ls 2>/dev/null | ls_normalize | grep -E '^term\.stdout 3 ' >/dev/null" >/dev/null

echo "--- terminal read term --limit 2 ---"
ppz_a terminal read term --limit 2
echo "--- terminal read --limit 2 term (flag-first order) ---"
ppz_a terminal read --limit 2 term
echo "--- terminal read term -l 2 (identical) ---"
ppz_a terminal read term -l 2
