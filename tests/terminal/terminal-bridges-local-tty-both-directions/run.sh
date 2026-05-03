#!/usr/bin/env bash
# `ppz terminal share` must bridge the user's local tty to the wrapped
# child both ways:
#   (a) local stdin → PTY master → child's stdin
#   (b) PTY master → local stdout → user sees the child's output
#
# Strategy: pipe a known string into ppz's stdin, run `head -n 1` as the
# child (reads one line, prints it back, exits). After it returns we
# expect the line to appear on:
#   (1) ppz's local stdout — proves direction (b)
#   (2) <handle>.stdout      — proves the publisher still runs in parallel
#
# `head -n 1` reading a line proves direction (a) implicitly: no input,
# no output.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

OUT=/tmp/term-bridge.out
rm -f "$OUT"

# Wrap with timeout: until the bridge is implemented, head -n 1 sees no
# input and would hang forever. timeout makes the failure observable.
timeout 10 sh -c '
  echo "hello-from-local-stdin" \
    | PPZ_IPC_SOCKET="'"$PPZ_DAEMON_A_SOCK"'" ppz terminal share term-bridge -- head -n 1
' >"$OUT" 2>/dev/null || true

local_count=$(grep -c hello-from-local-stdin "$OUT" 2>/dev/null || echo 0)
[ "$local_count" -ge 1 ] && echo "local-bridge-ok=yes" || echo "local-bridge-ok=no"

channel_count=$(ppz_a read term-bridge.stdout 2>/dev/null | grep -c hello-from-local-stdin || echo 0)
[ "$channel_count" -ge 1 ] && echo "channel-publish-ok=yes" || echo "channel-publish-ok=no"
