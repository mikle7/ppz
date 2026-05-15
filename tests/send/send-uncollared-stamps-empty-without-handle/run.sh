#!/usr/bin/env bash
# Phase 1.5.3 sender identity (revised): uncollared sends with NO
# current handle set still stamp envelope.sender = "" (anonymous).
# Companion to send-uncollared-stamps-current-handle which covers
# the with-handle case.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a unset namespace >/dev/null
ppz_a pipe create room >/dev/null

err=$(mktemp)
ppz_a send room "anonymous shout" 2>"$err"
grep -oE '^sent id=[a-f0-9]{8} to=[^ ]+ bytes=[0-9]+$' "$err" | head -1 \
  | sed -E 's/id=[a-f0-9]{8}/id=ID8/; s/bytes=[0-9]+/bytes=N/'

ppz_a reread room -l 1 --json | head -1 \
  | sed -E 's/.*"sender":"([^"]*)".*/sender=\1/'
rm -f "$err"
