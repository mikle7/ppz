#!/usr/bin/env bash
# `ppz await --tty` against a non-stdout pipe: write a warning to
# stderr but still honor the flag (the user explicitly asked for tty
# rendering). Stdout shows the vt10x-rendered bytes; stderr carries
# the warning.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a send chat.inbox "tty on inbox" >/dev/null
wait_for 20 "ppz_a ls | grep -q 'tty on inbox'" >/dev/null

ERR=/tmp/await-tty-warn.err
ppz_a await --tty chat.inbox 2>"$ERR" >/dev/null || true

if grep -qE '\-\-tty is only meaningful for stdout-shape pipes' "$ERR"; then
  echo "WARNING_ON_STDERR=yes"
else
  echo "WARNING_ON_STDERR=no"
fi
