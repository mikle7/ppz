#!/usr/bin/env bash
# `ppz await` with no args and no current handle set → E_NO_CURRENT_SOURCE.
# Default pattern `inbox` is resolved to `<current>.inbox`, which can't
# be evaluated without a current.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1

# Should exit non-zero with E_NO_CURRENT_SOURCE on stderr. We capture
# stderr explicitly so the assertion isn't satisfied by an unrelated
# "unknown command" exit-2 (which would happen if the verb didn't
# exist at all).
ERR=/tmp/await-nocurrent.err
ppz_a await 2>"$ERR"
rc=$?
echo "rc=$rc"
if grep -qE 'E_NO_CURRENT_SOURCE' "$ERR"; then
  echo "ERROR_CODE=E_NO_CURRENT_SOURCE"
else
  echo "ERROR_CODE=other"
fi
