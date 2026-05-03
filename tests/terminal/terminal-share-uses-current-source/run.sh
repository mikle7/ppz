#!/usr/bin/env bash
# Bare `ppz terminal share` (no handle) uses the current source. If the
# source doesn't yet have stdin/stdout pipes (e.g. it was created by
# `connect`, which only auto-provisions broadcast), wrap creates them
# transparently via the same code path as `pipe create`.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null

# Bare wrap with an explicit child command so the test doesn't hang.
ppz_a terminal share -- printf "wrapped" >/dev/null

# Wait for the byte-faithful chunk to land on chat.stdout.
wait_for 20 "ppz_a reread chat.stdout --json | jq -r '.payload' | tr -d '\r' | grep -q wrapped" >/dev/null

# stdout pipe should exist (auto-created by wrap), and we should be able
# to read what the wrapped child wrote.
ppz_a reread chat.stdout | tr -d '\r' | sed '/^$/d'
