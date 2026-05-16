#!/usr/bin/env bash
# `ppz await --tty` against a stdout-shape pipe is the canonical use
# case and emits NO warning. We use `terminal create` to provision a
# pty source (auto-pipe: stdout), publish some bytes to its stdout
# stream via `ppz send`, then await --tty and assert no warning hit
# stderr.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create term1 >/dev/null
# Inject bytes into the stdout stream so there's something to drain.
ppz_a send term1.stdout "hello-stdout" >/dev/null
wait_for 20 "ppz_a ls | grep -q hello-stdout" >/dev/null

ERR=/tmp/await-tty-silent.err
OUT=/tmp/await-tty-silent.out
ppz_a await --tty term1.stdout >"$OUT" 2>"$ERR" || true

# Two assertions: (1) no warning on stderr; (2) the drain actually
# happened — stdout contains the vt10x-rendered bytes we published.
# The second guards against the false-positive case where the verb
# doesn't exist at all and emits no warning simply because nothing ran.
if grep -qE '\-\-tty is only meaningful' "$ERR"; then
  echo "WARNING_ON_STDERR=yes"
else
  echo "WARNING_ON_STDERR=no"
fi
if grep -q 'hello-stdout' "$OUT"; then
  echo "DRAINED=yes"
else
  echo "DRAINED=no"
fi
