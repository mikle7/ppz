#!/usr/bin/env bash
# `ppz ls` NAMESPACE column for an uncollared pipe created locally
# with session namespace = "xyz": NAMESPACE shows "xyz", PIPE shows
# the bare pipe name.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle    >/dev/null 2>&1
ppz_a set namespace xyz >/dev/null
ppz_a pipe create plaza >/dev/null
ppz_a unset namespace >/dev/null

ppz_a ls | awk '$2 == "plaza" {print "namespace=" $1 " pipe=" $2}'
