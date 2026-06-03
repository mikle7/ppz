#!/usr/bin/env bash
# `ls --watch alice` (bare handle, no such pipe) warns but still BLOCKS —
# a warning, not an error, so you can pre-arm a watch on a pipe that
# doesn't exist yet. The glob form `alice.*` fires on alice's pipes.
. /tests/lib/common.sh
cleanup() { kill "$WPID" 2>/dev/null || true; wait "$WPID" 2>/dev/null || true; }
trap cleanup EXIT
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create alice >/dev/null
ppz_a send alice.inbox hi >/dev/null
wait_for 20 "ppz_a ls 'alice.*' | grep -q hi" >/dev/null
out=$(mktemp); err=$(mktemp)
ppz_a ls --watch alice >"$out" 2>"$err" &
WPID=$!
sleep 0.8
grep -qi 'no pipe matches' "$err" && echo "warned=yes" || echo "warned=no"
kill -0 "$WPID" 2>/dev/null && echo "blocked=yes" || echo "blocked=no"
kill "$WPID" 2>/dev/null || true; wait "$WPID" 2>/dev/null || true; WPID=
echo "--- glob form fires ---"
ppz_a ls --watch 'alice.*' | ls_normalize | awk '$1=="alice.inbox"{print $1, $2}'
