#!/usr/bin/env bash
# Manifold-uncollared subject shape is "<acct>.<manifold>.<pipe>" — three
# parts. The pre-fix callback misparsed this as handle=manifold,
# pipe=<pipe>, so pattern matching never saw the real (manifold, pipe).
# This test confirms the subscriber wakes correctly on namespaced
# uncollared traffic.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1
ppz_a set namespace team-a >/dev/null
ppz_a pipe create chat >/dev/null
ppz_a unset namespace >/dev/null

WATCH_OUT=/tmp/ls-watch-ns.out
rm -f "$WATCH_OUT"
ppz_a ls --watch > "$WATCH_OUT" 2>&1 &
WPID=$!
sleep 0.4

ppz_a set namespace team-a >/dev/null
ppz_a send chat "namespaced hello" >/dev/null
ppz_a unset namespace >/dev/null

for _ in $(seq 1 20); do
  if ! kill -0 "$WPID" 2>/dev/null; then break; fi
  sleep 0.5
done
wait "$WPID" 2>/dev/null || true

echo "--- watch fired on namespaced uncollared message ---"
ls_normalize < "$WATCH_OUT" | grep -E '^team-a\.chat '
