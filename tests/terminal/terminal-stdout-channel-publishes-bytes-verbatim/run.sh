#!/usr/bin/env bash
# The .stdout channel captures the PTY master's byte stream verbatim — no
# line splitting, no \n insertion. Escape sequences (the whole reason
# .stdout exists) make it through untransformed.
#
# `printf "X\033[31mY"` writes exactly 7 bytes: X, ESC (0x1b), [, 3, 1,
# m, Y — and crucially no trailing newline, so the PTY's cooked mode
# can't help us inflate the result. The wrapped child exits immediately;
# we then concatenate every .stdout chunk's payload and assert the byte
# sequence matches.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal share stdout-test -- sh -c 'printf "X\033[31mY"' >/dev/null

# Drain .stdout chunks. Payloads contain no newlines (printf emitted none),
# so jq -r + tr -d '\n' safely concatenates without losing data.
wait_for 20 "ppz_a reread stdout-test.stdout --json | jq -r '.payload' | tr -d '\n' | grep -q '\\['" >/dev/null

expected=$(printf "X\033[31mY" | xxd | head -1)
got=$(ppz_a reread stdout-test.stdout --json | jq -r '.payload' | tr -d '\n' | xxd | head -1)
[ "$expected" = "$got" ] && echo "stdout-bytes-ok=yes" || {
  echo "stdout-bytes-ok=no"
  echo "expected: $expected"
  echo "got:      $got"
}
