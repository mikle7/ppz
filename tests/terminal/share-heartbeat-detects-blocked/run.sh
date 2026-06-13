#!/usr/bin/env bash
# Phase 3 of agent detection (docs/specs/agent-detection.md): a harness
# that stops streaming and shows a permission dialog must surface as
# online|blocked — the state that means a human needs to go answer
# something. The stub walks the full pipeline: working output feeds the
# live vt10x screen model via the output tee; when byte causality goes
# quiet the detector consults the screen, sees the dialog, and the
# wake-beat flips `ppz who` to blocked.
. /tests/lib/common.sh

cleanup() {
  kill "$PID_CLAUDE" 2>/dev/null || true
  wait "$PID_CLAUDE" 2>/dev/null || true
}
trap cleanup EXIT

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

stubdir="$(mktemp -d)"
# Timeline: quiet through the 3s startup grace; ~6s of steady output
# (working); then draw a Claude-style permission dialog (❯ printed as
# octal \342\235\257 for POSIX printf) and sit on it — output stops, the
# dialog stays on the visible screen, detection must flip to blocked.
cat > "$stubdir/claude" <<'EOF'
#!/bin/sh
sleep 4
i=0
while [ "$i" -lt 30 ]; do
  echo "thinking $i"
  i=$((i+1))
  sleep 0.2
done
printf 'Do you want to proceed?\r\n'
printf '\342\235\257 1. Yes\r\n'
printf '  2. No, and tell Claude what to do differently (esc)\r\n'
sleep 60
EOF
chmod +x "$stubdir/claude"

PATH="$stubdir:$PATH" ppz_a terminal share det-blocked -- claude </dev/null >/dev/null &
PID_CLAUDE=$!

# Poll-and-capture (same pattern as share-heartbeat-detects-harness):
# the working phase is transient, so assert on the captured row.
poll_who_row() { # $1=handle $2=want "<status> <harness>" $3=attempts
  local row="" i
  for ((i = 0; i < $3; i++)); do
    row="$(ppz_a who | awk -v h="$1" '$1==h {print $2, $3}')"
    [[ "$row" == "$2" ]] && break
    sleep 0.1
  done
  echo "$row"
}

echo "working: $(poll_who_row det-blocked 'online|working claude' 120)"
echo "blocked: $(poll_who_row det-blocked 'online|blocked claude' 120)"
