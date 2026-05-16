#!/usr/bin/env bash
# `ppz await A B` watches both targets, OR-combined. Message lands on
# the second target only — await wakes, drains it, exits. The first
# target (inbox) has no unread and is not drained.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a unset handle >/dev/null 2>&1
ppz_a pipe create plaza >/dev/null
ppz_a send plaza "from plaza" >/dev/null
wait_for 20 "ppz_a ls | grep -q 'from plaza'" >/dev/null

# Re-set current so 'inbox' resolves; only plaza has unread, so plaza wins.
ppz_a set handle chat >/dev/null
ppz_a await inbox plaza
