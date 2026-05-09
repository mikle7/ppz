#!/usr/bin/env bash
# RED for Phase 0 (docs/AGENT_HARDENING.md §"Phase 0 — Observability"):
# `ppz status` must include a `nats:` line surfacing the daemon's NATS
# connection state. Today the daemon registers no DisconnectErrHandler /
# ReconnectHandler / ClosedHandler on `nats.Connect` and the status
# output has no nats field — so the assertion below diffs to nothing
# until Phase 0 ships.
#
# When green: at least one line of the form
#   nats: connected
# (further fields after `connected` — e.g. last drop timestamp, attempt
# counter — are tolerated; this scenario only pins the prefix so the
# Phase 0 implementation is free to evolve the detail.)
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

# Extract just the connection state token; drop any trailing detail.
ppz_a status | grep -oE "^nats: (connected|disconnected|connecting)" | head -1
