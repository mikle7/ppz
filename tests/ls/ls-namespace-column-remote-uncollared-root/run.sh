#!/usr/bin/env bash
# Remote-created uncollared pipe at root: NAMESPACE column on daemon A's
# `ppz ls` view of daemon B's pipe must render "-" (root), regardless
# of daemon A's own session namespace.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)"  >/dev/null
ppz_b daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha2)" >/dev/null

ppz_b unset namespace >/dev/null 2>&1
ppz_b unset handle    >/dev/null 2>&1
ppz_b pipe create plaza >/dev/null

wait_for 20 "ppz_a ls --json | grep -q '\"plaza\"'" >/dev/null

ppz_a ls | awk '$2 == "plaza" {print "namespace=" $1 " pipe=" $2}'
