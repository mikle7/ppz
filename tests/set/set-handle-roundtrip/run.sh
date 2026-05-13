#!/usr/bin/env bash
# `ppz set handle HANDLE` mutates the daemon's current-handle state;
# `ppz get handle` reads it back. Locked decision #20.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create alpha-h >/dev/null
ppz_a source create beta-h >/dev/null
# After two creates, daemon current is beta-h (latest wins).

echo "--- get handle (after two creates) ---"
ppz_a get handle

echo "--- set handle alpha-h ---"
ppz_a set handle alpha-h

echo "--- get handle (after set) ---"
ppz_a get handle
