#!/usr/bin/env bash
# Phase 2 channel set: broadcast / stdin / stdout. Anything else → exit 20.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a send chat.weirdchannel "hi"
