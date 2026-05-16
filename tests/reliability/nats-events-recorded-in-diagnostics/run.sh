#!/usr/bin/env bash
# Pins two contracts together (so they evolve together): the daemon
# registers NATS connection-event handlers AND surfaces the recorded
# events through `ppz diagnostics`.
#
# History: this verb was originally `ppz diag` (Phase 0 of
# docs/AGENT_HARDENING.md §"Phase 0 — Observability"); renamed to
# `ppz diagnostics` so the spelling matches the rest of the operator-
# facing surface — no two-letter mystery abbreviations.
#
# When green: `ppz diagnostics` lists at least one disconnect and one
# reconnect event after a brief NATS outage. Each event line begins
# with the event type token; further fields (timestamp, attempt
# count, error string) are not pinned here.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a send chat.inbox "before drop" >/dev/null

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
# user-visible signal that recovery completed. Both stdout (the table
# itself, which prints once `ls` succeeds) and stderr (errors during
# the polling loop) need silencing — we're using `ls` purely as a
# health probe; only the diagnostics output below matters for the
# assertion.
wait_for 600 'ppz_a ls >/dev/null 2>&1'

# `ppz diagnostics` must list both events. Print just the event-type
# tokens (sorted, deduped) so the assertion stays insensitive to
# ordering and detail.
ppz_a diagnostics 2>/dev/null | grep -oE "^(disconnect|reconnect)" | sort -u
