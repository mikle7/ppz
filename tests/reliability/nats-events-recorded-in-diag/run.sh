#!/usr/bin/env bash
# RED for Phase 0 (docs/AGENT_HARDENING.md §"Phase 0 — Observability"):
# the daemon must register NATS connection-event handlers AND surface
# the recorded events through a `ppz diag` verb. Pinning both contracts
# in one scenario so they evolve together.
#
# Today: no `ppz diag` verb exists; no DisconnectErrHandler /
# ReconnectHandler are registered. So the assertion below produces
# empty output (or an error from the unknown command) until Phase 0
# ships.
#
# When green: `ppz diag` lists at least one disconnect and one
# reconnect event after a brief NATS outage. Each event line begins
# with the event type token; further fields (timestamp, attempt
# count, error string) are not pinned here.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a broadcast -m "before drop" >/dev/null

# Brief NATS outage: stop ppz-server (which embeds NATS) for a few
# seconds, then restart. The default Go NATS client retries every 2s
# with a 60-attempt cap, so a 3s outage is well within the existing
# behaviour — we're not exercising long-outage recovery here, just
# that the disconnect / reconnect *events* are observable.
docker stop compose-ppz-server-1 >/dev/null
sleep 3
docker start compose-ppz-server-1 >/dev/null

# Wait for the daemon to be reachable to NATS again. `ppz ls` round-
# trips through NATS on every call, so its success is the canonical
# user-visible signal that recovery completed.
wait_for 600 'ppz_a ls 2>/dev/null'

# `ppz diag` must list both events. Print just the event-type tokens
# (sorted, deduped) so the assertion stays insensitive to ordering
# and detail.
ppz_a diag 2>/dev/null | grep -oE "^(disconnect|reconnect)" | sort -u
