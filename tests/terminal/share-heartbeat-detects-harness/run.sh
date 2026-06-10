#!/usr/bin/env bash
# Heartbeats must auto-detect an agent harness running inside a shared
# terminal — without `ppz agent`'s env vars — and stamp live state, so
# `ppz who` shows online|working / online|idle for hand-launched
# harnesses (docs/specs/agent-detection.md, phases 1+2).
#
# The detector identifies by foreground process name, so a stub
# `claude` on PATH exercises the whole production pipeline: TIOCGPGRP
# foreground inspection → byte-causality state → wake-beat on
# transition → daemon heartbeat cache → who renderer. The stub's
# timeline is built around the detection constants: quiet through the
# 3s startup grace, ~4s of steady output (working), then silence long
# enough for the 1800ms activity window to lapse (idle).
. /tests/lib/common.sh

cleanup() {
  kill "$PID_CLAUDE" "$PID_SHELL" 2>/dev/null || true
  wait "$PID_CLAUDE" "$PID_SHELL" 2>/dev/null || true
}
trap cleanup EXIT

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

stubdir="$(mktemp -d)"
cat > "$stubdir/claude" <<'EOF'
#!/bin/sh
sleep 4
i=0
while [ "$i" -lt 20 ]; do
  echo "thinking $i"
  i=$((i+1))
  sleep 0.2
done
# Stay in the PTY foreground so detection holds claude/idle instead of
# clearing when the child exits.
sleep 60
EOF
chmod +x "$stubdir/claude"

PATH="$stubdir:$PATH" ppz_a terminal share det-claude -- claude </dev/null >/dev/null &
PID_CLAUDE=$!
# Control row: a plain non-harness child keeps the bare liveness word
# and an empty harness column.
ppz_a terminal share det-shell -- sleep 60 </dev/null >/dev/null &
PID_SHELL=$!

# Poll-and-capture instead of wait_for: the working phase is transient
# (~4s), so assert on the captured row rather than re-querying after
# the wait and racing the working→idle transition.
poll_who_row() { # $1=handle $2=want "<status> <harness>" $3=attempts
  local row="" i
  for ((i = 0; i < $3; i++)); do
    row="$(ppz_a who | awk -v h="$1" '$1==h {print $2, $3}')"
    [[ "$row" == "$2" ]] && break
    sleep 0.1
  done
  echo "$row"
}

echo "working: $(poll_who_row det-claude 'online|working claude' 120)"
echo "idle: $(poll_who_row det-claude 'online|idle claude' 120)"
echo "shell: $(poll_who_row det-shell 'online -' 50)"

# --json keeps machine-readable fields split: top-level status is bare
# liveness, detection metadata rides the heartbeat payload.
ppz_a who --json | grep -A2 '"handle": "det-claude"' | grep -o '"status": "online"'
ppz_a who --json | grep -o '"harness_source": "detected"' | sort -u
