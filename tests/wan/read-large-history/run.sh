#!/usr/bin/env bash
# RED: ppz read on a stream with N retained messages must NOT scale
# linearly with N under WAN latency.
#
# Setup: 200 numbered messages on wantest.broadcast, published from
# daemon-b (no latency overlay). Read from daemon-a (200ms egress
# delay to ppz-server). The current historical-drain loop in
# handleRead does one stream.GetMsg(seq) per message, so total
# wall-time ≈ N × RTT (~40s for N=200, RTT=200ms). The fix is to
# batch-fetch via OrderedConsumer.
#
# Assertions cover both performance AND correctness so a "fast but
# lossy / out-of-order / duplicating" implementation can't pass:
#   - count == 200            (no message lost)
#   - first == msg-001        (start of range)
#   - last == msg-200         (end of range)
#   - sort -c                 (in order — no reorder)
#   - sort -u count == 200    (no duplicates)
#   - wall-time < 5s          (catches the linear-RT loop, broken ≈ 40s)
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_b daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

# Populate from daemon-b (no latency). Single ppz broadcast call
# with line-streaming reads stdin and publishes each line as one
# message — much faster than 200 separate CLI invocations.
ppz_b source create wantest >/dev/null 2>&1 || true
ppz_b source switch wantest >/dev/null
# busybox seq has no -f; use awk for the formatted output.
awk 'BEGIN{for(i=1;i<=200;i++)printf("msg-%03d\n",i)}' | ppz_b broadcast >/dev/null

# Wait until all 200 messages are visible to daemon-b's view of the
# stream. Using ls's BUFFERED column — when it hits 200 we know the
# server has retained them.
wait_for 100 "ppz_b ls 2>/dev/null | awk '/^wantest.broadcast/ { exit (\$3 == 200) ? 0 : 1 }'"

OUT=/tmp/wan-read-output.txt

# Time the read on daemon-a (under WAN latency). Default mode prints
# one message per line, which is what we want for the count / order
# assertions below. (--raw concatenates the bytes verbatim, which
# would squish all 200 messages into a single line because broadcast
# strips the trailing \n on each scanned input line.)
#
# Use bash's EPOCHREALTIME for sub-second precision — busybox `date`
# doesn't expand %N. Truncate to integer seconds for the assertion.
start_s="$EPOCHREALTIME"
ppz_a reread --bare wantest.broadcast >"$OUT" 2>/dev/null
end_s="$EPOCHREALTIME"
elapsed_s=$(awk -v a="$end_s" -v b="$start_s" 'BEGIN{printf "%.0f", a-b}')

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
