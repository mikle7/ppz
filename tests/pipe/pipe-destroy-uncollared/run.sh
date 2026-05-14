#!/usr/bin/env bash
# Phase 1.5: `ppz pipe destroy LEAF` works for uncollared (sourceless)
# pipes — same shape as the create/send/read paths. With no current
# handle, the CLI passes BareTarget=LEAF and the daemon resolves it
# as an uncollared pipe at the session's current namespace (root in
# this test).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1
ppz_a pipe create plaza >/dev/null
ppz_a pipe destroy plaza
