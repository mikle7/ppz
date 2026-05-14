#!/usr/bin/env bash
# Phase 1.5.1 first-wins collision rule: within a manifold, user-typed
# names share a namespace across source-handles and uncollared-pipe-
# names. Source `foo` exists → can't create uncollared pipe `foo` at
# same manifold.
#
# Asserts: source create succeeds, conflicting uncollared create errors
# with E_NAME_TAKEN (exit code in errors.go; numerical pin in expected
# checks the existing E_PIPE_TAKEN family — Phase 1.5.1 may introduce a
# more specific code, in which case this test gets updated).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a source create foo >/dev/null
ppz_a unset handle >/dev/null
ppz_a pipe create foo
echo "exit2=$?"
