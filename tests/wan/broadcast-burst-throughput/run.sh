#!/usr/bin/env bash
# RED: a burst of N broadcasts must NOT scale linearly with N under
# WAN latency.
#
# This is the publish-side analogue of read-large-history. handleBroadcast
# in the daemon issues TWO synchronous NATS round-trips per call:
#   1. js.Stream(...) lookup to validate the stream exists
#   2. d.NC.Flush() to wait for publish ack
# At 200ms RTT that's ~400ms per broadcast, so the drain loop in
# `terminal share`'s publishAndDisplayStdout (which is structurally
# `for payload := range publishCh { sendStreamLine(...) }` — sequential
# blocking IPCs) collapses to ~2.5 msg/sec under WAN. A 100-line burst
# from a wrapped child takes ~40s before subscribers see the last byte.
#
# Driving the loop directly via `ppz broadcast` avoids the PTY scheduling
# noise — sendStreamLine *is* daemon.Call(IPCBroadcast) under the hood,
# so this targets the same code path.
#
# Assertions:
#   - count == 100             (no message lost)
#   - first == bcast-001       (start of range)
#   - last == bcast-100        (end of range)
#   - sort -c                  (in order)
#   - sort -u count == 100     (no duplicates)
#   - wall-time < 5s           (catches the 2-RTT-per-broadcast loop;
#                               broken ≈ 40s)
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_b daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

# Source on daemon-a's side. Receiver (daemon-b) reads the same
# stream via the shared org's NATS account.
ppz_a source create wantest >/dev/null 2>&1 || true
ppz_a source switch wantest >/dev/null

OUT=/tmp/wan-burst-output.txt

start_s="$EPOCHREALTIME"
# Burst of 100 broadcasts via line-streaming stdin — exercises the
# same per-message daemon.Call path that publishAndDisplayStdout uses
# from `terminal share`.
awk 'BEGIN{for(i=1;i<=100;i++)printf("bcast-%03d\n",i)}' | ppz_a broadcast >/dev/null
end_s="$EPOCHREALTIME"
elapsed_s=$(awk -v a="$end_s" -v b="$start_s" 'BEGIN{printf "%.0f", a-b}')

# Wait until daemon-b sees all 100 retained.
wait_for 100 "ppz_b ls 2>/dev/null | awk '/^wantest.broadcast/ { exit (\$3 == 100) ? 0 : 1 }'"

# Pull them — daemon-b has no WAN delay, so this is fast and not
# what we're measuring. We use it purely for the correctness checks
# below.
ppz_b reread --bare wantest.broadcast >"$OUT" 2>/dev/null

count=$(wc -l <"$OUT" | tr -d ' ')
first=$(head -n 1 "$OUT")
last=$(tail -n 1 "$OUT")
unique=$(sort -u "$OUT" | wc -l | tr -d ' ')
sort -c "$OUT" 2>/dev/null && in_order=true || in_order=false

echo "count: $count"
echo "first: $first"
echo "last: $last"
echo "unique: $unique"
echo "in_order: $in_order"
[[ $elapsed_s -lt 5 ]] && echo "under_5s: yes" || echo "under_5s: no (${elapsed_s}s)"
