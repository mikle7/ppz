#!/usr/bin/env bash
# `ppz read foo` (no .channel suffix) is rejected with E_INVALID_CHANNEL
# (exit 20). The implicit form `pipes read <handle>` from the original spec
# is intentionally not implemented in Phase 1.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create foo >/dev/null
ppz_a read foo
