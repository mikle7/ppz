#!/usr/bin/env bash
# RED for the synchronous-ack-emit regression: a `ppz read` that
# triggers ack auto-emission must NOT scale with the number of ack-
# requested messages drained, even under WAN latency.
#
# Setup:
#   - daemon-b (no latency): publishes N --request-ack messages to
#     alpha-side.inbox. Sender id is beta-side, so each message
#     legitimately needs an ack back to beta-side.inbox.
#   - daemon-a (200ms egress delay): owns alpha-side, runs `ppz read`,
#     and is therefore the one whose daemon emits the N acks.
#
# Each ack publish is one NATS RTT through daemon-a's shaped egress
# (~200ms). With a synchronous ack-emit loop the read RPC blocks
# for N × RTT before returning to the CLI (~10s for N=50). With the
# fix (goroutine-detached emit, advance-then-emit ordering preserved)
# the read returns once the cursor advances — bounded by the
# historical-drain RTT (~200ms).
#
# Assertions:
#   - read_msg_count == N            (no message lost on the read path)
#   - read_under_5s == yes           (catches the synchronous loop;
#                                     broken ≈ 10s)
#   - ack_count == N                 (eventually — acks DO fire,
#                                     just in the background)
#
# Picking N=50: synchronous = 10s vs detached = ~0.2s gives a 50×
# margin and stays well clear of the 5s threshold without bloating
# setup time (50 sequential `ppz send` invocations on the no-latency
# side are ~5s of wall-time).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)"  >/dev/null
ppz_b daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha2)" >/dev/null

# Receiver source (daemon-a, under WAN). Sender source (daemon-b,
# no latency) — needs a current source of its own so --request-ack
# preflight passes and the ack has a destination.
ppz_a source create alpha-side >/dev/null
ppz_b source create beta-side  >/dev/null

# Publish N --request-ack messages from daemon-b to alpha-side.inbox.
# Sequential because --request-ack only applies to single sends, not
# the streaming broadcast batch path. Each call is sub-ms on daemon-b's
# NATS path (no latency); the wall-time is dominated by CLI process
# spawn overhead (~50ms each → ~2.5s total for N=50).
N=50
for i in $(seq 1 $N); do
  ppz_b send alpha-side "msg-$(printf %03d $i)" --request-ack 2>/dev/null
done

# Wait until daemon-a's view of the stream shows all N messages
# buffered (BUFFERED column on `ls` lines like `alpha-side.inbox 50 50 ...`).
wait_for 60 "ppz_a ls 2>/dev/null | awk '/^alpha-side.inbox/ { exit (\$2 == $N) ? 0 : 1 }'"

OUT=/tmp/wan-acked-read-output.txt

# Time the read on daemon-a. If ack emission is synchronous on the
# read path, this is dominated by N × WAN-RTT (~200ms each). With the
# detached-goroutine fix it's bounded by the historical-drain time
# (~one batched Fetch RTT).
start_s="$EPOCHREALTIME"
ppz_a read --bare alpha-side.inbox >"$OUT" 2>/dev/null
end_s="$EPOCHREALTIME"
elapsed_s=$(awk -v a="$end_s" -v b="$start_s" 'BEGIN{printf "%.0f", a-b}')

read_msg_count=$(wc -l <"$OUT" | tr -d ' ')

# Eventually, all N acks land in beta-side.inbox. They fire from
# detached goroutines on daemon-a, each costing ~one WAN-RTT for the
# Publish + Flush, so under N=50 with concurrent goroutines the NATS
# connection pipelines them into a small handful of round-trips. We
# allow a generous timeout so the test isn't flaky on slow CI.
wait_for 120 "ppz_b ls 2>/dev/null | awk '/^beta-side.inbox/ { exit (\$2 == $N) ? 0 : 1 }'"
ack_count=$(ppz_b ls 2>/dev/null | awk '/^beta-side.inbox/ { print $2 }')

echo "read_msg_count: $read_msg_count"
[[ $elapsed_s -lt 5 ]] && echo "read_under_5s: yes" || echo "read_under_5s: no (${elapsed_s}s)"
echo "ack_count: $ack_count"
