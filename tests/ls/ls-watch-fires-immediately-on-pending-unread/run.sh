#!/usr/bin/env bash
# `ppz ls --watch` is level-triggered: if the calling session already has
# unread on a matching pipe, it returns the snapshot immediately without
# waiting for new traffic. Critical for the "agent restarts and shouldn't
# block forever waiting for fresh messages it could already process"
# case.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a broadcast -m "preexisting" >/dev/null
wait_for 20 "ppz_a ls 2>/dev/null | grep -q preexisting" >/dev/null

# Watch should return immediately because chat has unread.
echo "--- watch returns immediately (level-triggered) ---"
ppz_a ls --watch | ls_normalize
