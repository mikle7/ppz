#!/usr/bin/env bash
# `subs wait --json` emits the identical per-row JSON shape as
# `ls --watch --json` (handle/pipe/total/unread/last_at/...), scoped to
# the unread subscribed row(s).
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
PPZ_SESSION=desk ppz_a source create foo >/dev/null
PPZ_SESSION=foo  ppz_a send foo.inbox "hi" >/dev/null
wait_for 20 "PPZ_SESSION=foo ppz_a subs ls | grep -q hi" >/dev/null
PPZ_SESSION=foo ppz_a subs wait --json \
  | jq -c '{handle, pipe, total, unread, has_last_at: (.last_at != null)}'
