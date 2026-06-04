#!/usr/bin/env bash
# `ppz ls --watch` shares the stale-cursor failure mode. After a source
# is destroyed + recreated, the new stream's seq sits below the old
# session cursor, so unread computes 0. The fresh publish DOES wake the
# watcher (it subscribes to the org subject space), but the post-wakeup
# snapshot is written unconditionally (handleListWatch re-checks
# hasUnread only on the *initial* snapshot), so watch returns a snapshot
# reporting unread=0 — the agent loop `while true; do ppz ls --watch |
# process; done` is told there's nothing to read and silently skips the
# new message.
#
# Correct: the recreated source is fresh, the new message is unread, so
# watch reports unread=1. The kill/budget below is a safety net in case
# a future change makes the watch block instead (empty output also FAILs).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create alex >/dev/null
ppz_a send alex.inbox "old-1" >/dev/null
ppz_a send alex.inbox "old-2" >/dev/null
ppz_a send alex.inbox "old-3" >/dev/null
wait_for 20 "ppz_a ls | grep -q old-3" >/dev/null
ppz_a read alex.inbox >/dev/null   # cursor -> 3

ppz_a source destroy alex >/dev/null
ppz_a source create alex >/dev/null

WATCH_OUT=/tmp/ls-watch-recreate.out
rm -f "$WATCH_OUT"
ppz_a ls --watch 'alex.*' > "$WATCH_OUT" 2>&1 &
WPID=$!
# Let the daemon register its NATS subscription before the trigger send.
sleep 0.4
ppz_a send alex.inbox "after-recreate" >/dev/null

# Watch should fire and exit within budget. If the bug is present it
# blocks forever; kill it after ~10s so the scenario still completes.
for _ in $(seq 1 20); do
  if ! kill -0 "$WPID" 2>/dev/null; then break; fi
  sleep 0.5
done
kill "$WPID" 2>/dev/null || true
wait "$WPID" 2>/dev/null || true

echo "--- watch fired after recreate ---"
ls_normalize < "$WATCH_OUT" | awk '$1 == "alex.inbox" {print "unread="$2, "buffered="$3}'
