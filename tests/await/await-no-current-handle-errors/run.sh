#!/usr/bin/env bash
# `ppz await` with no args and no current handle set → E_NO_CURRENT_SOURCE.
# Default pattern `inbox` is resolved to `<current>.inbox`, which can't
# be evaluated without a current.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1

# Should exit non-zero with E_NO_CURRENT_SOURCE on stderr (stderr is
# discarded by the harness; we assert exit code only).
ppz_a await
