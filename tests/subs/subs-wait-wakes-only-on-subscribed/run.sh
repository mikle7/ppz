#!/usr/bin/env bash
# `subs wait` is `ls --watch` scoped to the sub list. Subscribed to
# foo.inbox only:
#   - a send to the UNSUBSCRIBED other.inbox must NOT wake it
#   - a send to foo.inbox wakes it; output is ONLY the unread row.
. /tests/lib/common.sh
cleanup() { kill "$WPID" 2>/dev/null || true; wait "$WPID" 2>/dev/null || true; }
trap cleanup EXIT
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
PPZ_SESSION=desk ppz_a source create foo   >/dev/null   # foo.inbox auto-subbed under "foo"
PPZ_SESSION=desk ppz_a source create other >/dev/null

OUT=$(mktemp)
PPZ_SESSION=foo ppz_a subs wait >"$OUT" 2>/dev/null &
WPID=$!
sleep 0.6   # let wait take its initial (caught-up) snapshot and block

# Unrelated traffic — must not wake foo's wait.
ppz_a send other.inbox "not-for-foo" >/dev/null
sleep 0.6
if kill -0 "$WPID" 2>/dev/null; then echo "unrelated=still-blocked"; else echo "unrelated=WOKE-BUG"; fi

# Subscribed traffic — wakes it.
ppz_a send foo.inbox "hello-foo" >/dev/null
wait "$WPID" 2>/dev/null; WPID=
echo "--- wait output (pipe unread) ---"
ls_normalize <"$OUT" | awk '{print $1, $2}'
