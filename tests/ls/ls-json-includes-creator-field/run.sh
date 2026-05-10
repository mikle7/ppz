#!/usr/bin/env bash
# `ppz ls --json` schema gains a `creator` key alongside `payload`.
# Every (source, pipe) row carries the username; on auto-provisioned
# pipes it equals the source's creator. The `payload` key remains
# (agent path still needs the bytes).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a broadcast -m "hi" >/dev/null
wait_for 20 "ppz_a ls | grep -q hi" >/dev/null

# Project (handle, pipe, creator, has_payload_key). `has_payload_key`
# pins that we DIDN'T accidentally drop the payload field while adding
# creator. `has` is true iff the key is present (even when empty-string).
ppz_a ls --json \
  | jq -c '{handle, pipe, creator, has_payload_key: has("payload")}'
