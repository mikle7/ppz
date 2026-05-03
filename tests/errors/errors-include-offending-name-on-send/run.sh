#!/usr/bin/env bash
# `ppz send <handle>.<pipe> PAYLOAD` against a non-existent source must
# name which source. The daemon's handleBroadcast has a second E_SOURCE_NOT_FOUND
# callsite (after the KnowsPipe refresh fails) that the original Bug 2 sweep
# missed — it returned generic "source not found" with no name.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

echo "--- E_SOURCE_NOT_FOUND on send to missing source ---"
ppz_a send phantom.broadcast "hi" 2>&1 | grep '^error:' || true
