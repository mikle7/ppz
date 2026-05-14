#!/usr/bin/env bash
# v0.31.1 — `ppz pipe create LEAF` (bare) under a set namespace must
# create an uncollared pipe at that manifold, ignoring any current
# handle.
#
# v0.31.0 ships with state-driven create routing: bare LEAF with a
# current handle set auto-collars under that handle, even when the
# user has explicitly set a namespace. The result is surprising:
#
#   ppz set namespace red-team
#   ppz pipe create room    →   foo.room   (collared under foo,
#                                            manifold '' inherited
#                                            from source row)
#
# The Phase 1.5.1 rule: current_handle is for sender identity, not
# destination routing. Bare creates always use current_namespace as
# manifold and create uncollared. To create a collared pipe, user
# types the dotted form `ppz pipe create HANDLE.LEAF`.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1
ppz_a unset namespace >/dev/null 2>&1

# Set up the conflict: source foo exists (sets current_handle=foo as
# a side effect of `ppz source create`) AND a namespace is set.
ppz_a source create foo >/dev/null
ppz_a set namespace red-team >/dev/null

# Bare create. Must go to red-team.room (uncollared at namespace),
# NOT foo.room (collared at root) — current_handle must not affect
# the destination.
ppz_a pipe create room
