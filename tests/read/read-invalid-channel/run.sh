#!/usr/bin/env bash
# Phase 1 only supports the broadcast channel. Any other suffix
# (.inbox / .std.in / .std.out / etc.) → E_INVALID_CHANNEL.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create foo >/dev/null
ppz_a read foo.inbox
