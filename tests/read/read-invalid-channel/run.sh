#!/usr/bin/env bash
# Unknown pipe names should still be rejected.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create foo >/dev/null
ppz_a read foo.nope
