#!/usr/bin/env bash
# `xxx | ppz broadcast` defaults to line-streaming: each line on stdin
# becomes its own message. Pipes are streaming-coded by Unix
# convention; the previous "atomic single message" behaviour
# deadlocked on `tail -f` and surprised users feeding any multi-line
# input. To preserve the atomic-block-send case, use `--eof`.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create foo >/dev/null

printf 'first\nsecond\nthird\n' | ppz_a broadcast >/dev/null
wait_for 20 "ppz_a ls | ls_normalize | grep -q '^foo.broadcast 3 3'" >/dev/null

echo "--- ls reports 3 buffered messages on foo.broadcast ---"
ppz_a ls | ls_normalize | grep '^foo.broadcast'

echo "--- reread emits one payload per line, in order ---"
ppz_a reread foo.broadcast
