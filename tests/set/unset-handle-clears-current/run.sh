#!/usr/bin/env bash
# `ppz unset handle` clears the daemon's current-handle state.
# Afterwards, `ppz get handle` exits non-zero with empty output so
# scripts capturing via $(ppz get handle) can detect "not set" cleanly.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create alpha-h >/dev/null

echo "--- before unset: handle is alpha-h ---"
ppz_a get handle

echo "--- unset ---"
ppz_a unset handle

echo "--- after unset: get exits non-zero, empty output ---"
out=$(ppz_a get handle 2>/dev/null)
rc=$?
echo "rc=$rc"
echo "out=[$out]"
