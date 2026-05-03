#!/usr/bin/env bash
# A wrapped terminal sets PPZ_CURRENT_HANDLE so its child's broadcasts
# target the wrap's pipe. If that pipe later disappears (deleted, or the
# user is in a stale shell), broadcasts must FAIL — not silently return
# success while messages drop into a non-existent JetStream stream.
#
# Reproduces: user reported `ppz broadcast "hello"` returned `sent id=...`
# even though the target handle wasn't a real pipe; ls confirmed nothing
# landed anywhere.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create real >/dev/null  # gives us a current; verifies login worked

# Aim broadcast at a handle that has never been created.
PPZ_CURRENT_HANDLE=ghost ppz_a broadcast -m "should fail"
