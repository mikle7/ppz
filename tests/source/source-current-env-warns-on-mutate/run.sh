#!/usr/bin/env bash
# When PPZ_CURRENT_HANDLE is set, mutating commands (source create /
# source switch / source clear) still do their daemon-side mutation
# but print a stderr warning that the env override means the mutation
# won't take effect until the user unsets it.
#
# Without the warning, "I just ran `ppz source switch foo` but my
# broadcasts are still going to bar" is a confusing afternoon.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create foo >/dev/null

echo "--- env set, source create new: daemon current updated, stderr warns ---"
PPZ_CURRENT_HANDLE=foo ppz_a source create new 2>&1 >/dev/null | grep '^warning:' || true

echo "--- env set, source switch foo: daemon current updated, stderr warns ---"
PPZ_CURRENT_HANDLE=foo ppz_a source switch foo 2>&1 >/dev/null | grep '^warning:' || true

echo "--- env set, source clear: daemon current cleared, stderr warns ---"
PPZ_CURRENT_HANDLE=foo ppz_a source clear 2>&1 >/dev/null | grep '^warning:' || true
