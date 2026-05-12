#!/usr/bin/env bash
# `ppz ls --json` is the agent-readable form: one JSON object per line
# (JSONL), full untruncated payload (the one place agents can ask "what
# was the last message?" without hitting `ppz read`), ISO timestamps.
#
# Schema per row: {handle, pipe, total, unread, last_at, payload}.
# last_at is null when the pipe has no messages; payload is "" likewise.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create chat >/dev/null
LONG="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
ppz_a send chat.inbox "$LONG" >/dev/null
wait_for 20 "ppz_a ls | grep -q aaaaaaaa" >/dev/null

# Validate each line is well-formed JSON, project a stable subset (drop
# last_at — it varies per run; keep a has_last_at boolean instead). Also
# verify the full payload — no truncation, no "…" marker.
ppz_a ls --json \
  | jq -c '{handle, pipe, total, unread, has_last_at: (.last_at != null), payload_len: (.payload | length), truncated: (.payload | endswith("…"))}'
