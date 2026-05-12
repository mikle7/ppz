#!/usr/bin/env bash
# RED: sending to a custom pipe that doesn't exist (no `ppz pipe create`
# was run on it) must surface as E_PIPE_NOT_FOUND — NOT E_INVALID_PIPE.
#
# Current behaviour (against main, 2026-05-12): the daemon's pre-publish
# stream-existence check returns the catch-all E_INVALID_PIPE for every
# js.Stream() failure, conflating "stream doesn't exist" (real not-found)
# with "couldn't reach server to check" (network failure). See the
# sibling scenario tests/reliability/send-with-server-down-returns-
# nats-unreachable for the network-failure half of the same routing
# bug.
#
# The routing fix needs to inspect the error from js.Stream() and pick:
#   - jetstream.ErrStreamNotFound  → E_PIPE_NOT_FOUND
#   - network / timeout            → E_NATS_UNREACHABLE
#   - genuinely unexpected         → E_INVALID_PIPE (catch-all)
#
# This scenario covers the first case — server is reachable, the
# stream just doesn't exist.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create chat >/dev/null

# `chat` exists; `chat.nope` does not. `nope` is a valid pipe name
# (passes natsubj.ValidatePipe regex), so we reach the JetStream
# stream-existence check where the bug lives.
err_code=$(ppz_a send chat.nope "test" 2>&1 | grep -oE "^error: E_[A-Z_]+" | head -1 | sed 's/^error: //')
echo "error_code=$err_code"
