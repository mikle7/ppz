#!/usr/bin/env bash
# `ppz read` is cursor-driven and rejects historical filter flags.
# -l / --skip / --since live on `ppz reread` (the forensic verb). The
# split keeps each verb single-purpose: read consumes, reread replays.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a broadcast -m "msg-1" >/dev/null
wait_for 20 "ppz_a ls | grep -q msg-1" >/dev/null

echo "--- read -l 2: should error, exit nonzero ---"
ppz_a read chat.broadcast -l 2 >/dev/null 2>&1
echo "rc=$?"

echo "--- read --skip 1: should error, exit nonzero ---"
ppz_a read chat.broadcast --skip 1 >/dev/null 2>&1
echo "rc=$?"

echo "--- read --since 1s: should error, exit nonzero ---"
ppz_a read chat.broadcast --since 1s >/dev/null 2>&1
echo "rc=$?"
