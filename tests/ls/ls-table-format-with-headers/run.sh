#!/usr/bin/env bash
# `ppz ls` is the agent + operator's primary status view. It must be:
#   - self-describing: a header row labels each column
#   - column-aligned: easy to scan visually
#   - human-friendly time: relative ("just now" / "5 minutes ago"), not
#     RFC3339; --iso flag exists for the agent-precise case.
#   - payload truncation marker: "…" suffix when a payload is cut at the
#     60-byte cap, so users know there's more available via `ppz read`.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a send chat.inbox "hello world" >/dev/null
ppz_a source create verbose >/dev/null
# A payload that exceeds the 60-byte preview cap so we can assert the … marker.
LONG="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
ppz_a send verbose.inbox "$LONG" >/dev/null
wait_for 20 "ppz_a ls | grep -q aaaaaaaa" >/dev/null

# Normalise variable-width whitespace to a single space, and the relative
# time to RELATIVE so the test isn't time-dependent.
ppz_a ls \
  | sed -E 's/[[:space:]]+/ /g' \
  | sed -E 's/(just now|[0-9]+ (seconds?|minutes?|hours?|days?) ago)/RELATIVE/'
