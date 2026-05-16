#!/usr/bin/env bash
# `ppz await` with no args and no current handle set → E_NO_CURRENT_SOURCE.
# Default pattern `inbox` is resolved to `<current>.inbox`, which can't
# be evaluated without a current.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1

# Should exit with the E_NO_CURRENT_SOURCE-mapped exit code (16) — not
# the generic exit-2 of an unknown-verb dispatch. Asserting on the
# specific rc avoids the false-positive case where the verb doesn't
# exist at all (which would dump usage text containing
# "E_NO_CURRENT_SOURCE" to stderr).
ppz_a await 2>/dev/null
echo "rc=$?"
