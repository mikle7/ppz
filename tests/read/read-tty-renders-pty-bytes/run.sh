#!/usr/bin/env bash
# `ppz read <h>.stdout --tty` is the read-side equivalent of `ppz terminal
# peek <h>` (which itself is sugar for this exact invocation under the
# hood). Both collect the captured PTY byte stream and run it through a
# virtual VT100 terminal, printing the rendered screen state — so ANSI
# escapes resolve into cell positions instead of leaking as visible junk.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

# Emit ESC[2J (clear screen) + ESC[1m (bold) + "rendered" + ESC[0m (reset).
# After vt10x rendering, only the literal text "rendered" should appear.
ppz_a terminal share term -- printf '\033[2J\033[1mrendered\033[0m' >/dev/null
wait_for 20 "ppz_a ls 2>/dev/null | ls_normalize | grep -E '^term\.stdout 1 ' >/dev/null" >/dev/null

ppz_a read term.stdout --tty
