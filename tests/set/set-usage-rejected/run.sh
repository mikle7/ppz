#!/usr/bin/env bash
# Each daemon-state verb has a small set of usage forms — passing
# the wrong number of args must exit 2 with a usage hint.
. /tests/lib/common.sh

echo "--- set (no args) ---"
err=$(ppz_a set 2>&1 1>/dev/null)
echo "rc=$?"
echo "$err" | grep -oE 'usage:' | head -1

echo "--- set handle (no value) ---"
err=$(ppz_a set handle 2>&1 1>/dev/null)
echo "rc=$?"
echo "$err" | grep -oE 'usage:' | head -1

echo "--- unset (no key) ---"
err=$(ppz_a unset 2>&1 1>/dev/null)
echo "rc=$?"
echo "$err" | grep -oE 'usage:' | head -1

echo "--- get (no key) ---"
err=$(ppz_a get 2>&1 1>/dev/null)
echo "rc=$?"
echo "$err" | grep -oE 'usage:' | head -1
