#!/usr/bin/env bash
# `ppz send <handle>.<channel> <payload>` writes one message to that
# subject. Targeting .broadcast on a regular pipe should be visible via
# `ppz read`.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a send chat.broadcast "explicit hello" >/dev/null

wait_for 20 "ppz_a ls | grep -q 'explicit hello'" >/dev/null
ppz_a read chat.broadcast
