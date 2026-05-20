#!/usr/bin/env bash
# Remote-created uncollared pipe in manifold "xyz": daemon A's ls view
# must show NAMESPACE="xyz" for that row even when daemon A's own
# session has no namespace set.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)"  >/dev/null
ppz_b daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha2)" >/dev/null

ppz_b unset handle    >/dev/null 2>&1
ppz_b set namespace xyz >/dev/null
ppz_b pipe create plaza >/dev/null
ppz_b unset namespace >/dev/null

ppz_a unset namespace >/dev/null 2>&1

wait_for 20 "ppz_a ls --json | grep -q '\"plaza\"'" >/dev/null

ppz_a ls | awk '$2 == "plaza" {print "namespace=" $1 " pipe=" $2}'
