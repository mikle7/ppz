#!/usr/bin/env bash
# v0.31.1 regression — `ppz send LEAF` for an uncollared pipe.
#
# v0.31.0 ships with a bug introduced in commit 005204d (the PR #42
# review fixes): shouldDispatchUncollared() rejected the dispatch when
# reqHandle was non-empty, but the CLI's legacy `.inbox` sugar always
# sets reqHandle for bare names. Result: bare-name sends to an
# uncollared pipe always took the collared path and surfaced
# E_SOURCE_NOT_FOUND.
#
# Asserts the user-facing behaviour: after `ppz pipe create LEAF`
# (uncollared, no current handle), `ppz send LEAF "msg"` succeeds and
# `ppz reread LEAF` reads the published payload back.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1
ppz_a unset namespace >/dev/null 2>&1
ppz_a pipe create room >/dev/null

# Send and capture exit code separately from the success line — the
# bug surfaces as a non-zero exit + an error to stderr. Normalising
# the success line follows the send-success-line-on-stderr fixture.
err=$(mktemp)
ppz_a send --from pubsub room "uncollared payload" 2>"$err"
echo "send-exit=$?"
grep -oE '^sent id=[a-f0-9]{8} to=[^ ]+ bytes=[0-9]+$' "$err" | head -1 \
  | sed -E 's/id=[a-f0-9]{8}/id=ID8/; s/bytes=[0-9]+/bytes=N/'

# Read the payload back to prove the publish reached the uncollared
# stream.
ppz_a reread room -l 1 --bare
rm -f "$err"
