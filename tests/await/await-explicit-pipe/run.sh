#!/usr/bin/env bash
# `ppz await <pipe>` against an uncollared pipe: buffered message is
# drained via the tabular renderer (uncollared pipes are messaging-shape).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1
ppz_a pipe create plaza >/dev/null
ppz_a send plaza "uncollared hello" >/dev/null
wait_for 20 "ppz_a ls | grep -q 'uncollared hello'" >/dev/null

ppz_a await plaza
