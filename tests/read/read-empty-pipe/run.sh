#!/usr/bin/env bash
# Pipe exists but no broadcasts yet → no stdout, exit 0.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create silent >/dev/null
ppz_a read silent.broadcast
