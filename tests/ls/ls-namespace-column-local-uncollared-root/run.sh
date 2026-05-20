#!/usr/bin/env bash
# `ppz ls` NAMESPACE column for a locally-created uncollared pipe at
# root namespace: NAMESPACE shows "-" (missing-value glyph), PIPE shows
# the bare pipe name with no manifold prefix.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset namespace >/dev/null 2>&1
ppz_a unset handle    >/dev/null 2>&1

ppz_a pipe create plaza >/dev/null

ppz_a ls | awk '$2 == "plaza" {print "namespace=" $1 " pipe=" $2}'
