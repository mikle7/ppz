#!/usr/bin/env bash
# `ppz pipe create <name>` (no flags) provisions a JetStream stream on the
# current source with the hardcoded defaults: 24 h max age, 1000 max msgs,
# 64 MiB max bytes. The new pipe is visible in `ppz ls`.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a pipe create archive
ppz_a ls | grep '^chat\.' | ls_normalize
