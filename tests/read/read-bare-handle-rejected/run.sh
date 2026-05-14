#!/usr/bin/env bash
# Phase 1.5: `ppz read foo` (bare, no dot) is now interpreted as a
# read against an uncollared pipe `foo` at the daemon's current
# namespace (root by default). Pre-1.5 this returned E_INVALID_PIPE
# (exit 20); now it returns E_PIPE_NOT_FOUND (exit 21) when no
# uncollared pipe with that name exists in the namespace.
#
# Test creates a source `foo` (the collared `foo.inbox` exists, but
# that's irrelevant — bare-name reads no longer auto-suffix .inbox)
# then clears the current handle so the read takes the uncollared
# path; no uncollared pipe at the root manifold called `foo` exists,
# so it reports E_PIPE_NOT_FOUND.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create foo >/dev/null
ppz_a unset handle >/dev/null
ppz_a read foo
