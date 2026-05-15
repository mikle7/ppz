#!/usr/bin/env bash
# Phase 1.5.2: `ppz read <handle>.stdout` defaults to --tty (vt10x
# screen render). Mirrors `ppz terminal read` which already auto-
# injects --tty. The mental model: stdout carries pty bytes and
# only makes sense after vt10x interpretation; raw escape codes
# leaking into the terminal are useless.
#
# Pre-1.5.2 behaviour: plain `read X.stdout` fell to the byte-faithful
# default, dumping ANSI escapes verbatim. RED until the runRead case
# selector recognises stdout as a tty-default channel.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

# Same setup as tests/read/read-tty-renders-pty-bytes: emit ESC[2J
# (clear screen) + ESC[1m (bold) + "rendered" + ESC[0m (reset).
# vt10x rendering strips the escapes and yields just "rendered".
ppz_a terminal share term -- printf '\033[2J\033[1mrendered\033[0m' >/dev/null
wait_for 20 "ppz_a ls 2>/dev/null | ls_normalize | grep -E '^term\.stdout 1 ' >/dev/null" >/dev/null

# No --tty flag. Should still render via vt10x because the target
# pipe is .stdout.
ppz_a read term.stdout
