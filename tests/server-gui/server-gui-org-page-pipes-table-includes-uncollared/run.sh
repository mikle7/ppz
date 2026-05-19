#!/usr/bin/env bash
# Bug: the org pipes tab on /orgs/<slug> excludes uncollared
# (sourceless) pipes. A user who runs `ppz pipe create <leaf>` at the
# account root (no current handle set) sees the row in `ppz ls` and
# can read/write it from the CLI, but the GUI table renders only
# pipes that hang off a source — uncollared rows are dropped on the
# server side before the template ever sees them.
#
# This scenario provokes the bug: create one uncollared pipe `plaza`
# at the account root, fetch the org page, and assert that a pipes-
# table row exists for it (via the stable data-source-row marker).
# Uncollared rows render with an empty handle slot, so the marker is
# `:plaza::` (handle="", pipe="plaza", no last message, no payload).
. /tests/lib/common.sh
auth_as_foo

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1
ppz_a unset namespace >/dev/null 2>&1

# Create an uncollared pipe at the account root. Bare LEAF with no
# namespace / no handle = uncollared at root (Phase 1.5 semantics).
ppz_a pipe create plaza >/dev/null

# Wait for the daemon to observe the new pipe via `ls` before hitting
# the GUI — the server-side row exists immediately after the API call,
# but this guard keeps the scenario robust to any cache/refresh lag.
wait_for 20 "ppz_a ls | grep -q plaza" >/dev/null

PAGE="$(curl_server "/orgs/alpha")"

# Only the plaza row — keep the assertion narrow so changes to other
# auto-provisioned rows don't churn this fixture.
echo "$PAGE" \
  | grep -oE 'data-source-row="[^"]*plaza[^"]*"' \
  | sed -E 's/data-source-row="([^"]+)"/\1/'
