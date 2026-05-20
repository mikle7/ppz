#!/usr/bin/env bash
# RC-D: share process killed -9 → no orphan binding stays alive.
# Lazy validation on next lookup drops the dead-pid entry. A
# subsequent in-pty-style call (which can't happen because the pty
# is gone, but a spurious call with the dead pid in ancestor_pids
# could happen if a sibling process is still alive) does not resolve
# to cindy.
#
# We can't easily synthesize an "ancestor_pid is X" call from the
# test, so we verify by: kill the share, observe the binding is no
# longer registered in the daemon's view (via diagnostics or via
# the in-mem agent-bindings.json reflecting the drop after a
# triggering lookup).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

HOME_S=/tmp/rc-d
rm -rf "$HOME_S"; mkdir -p "$HOME_S"
SOCK_S=$HOME_S/daemon.sock

ppz_s() { PPZ_HOME=$HOME_S PPZ_IPC_SOCKET=$SOCK_S ppz "$@"; }

cleanup() {
  PID=$(cat "$HOME_S/daemon.pid" 2>/dev/null || true)
  [[ -n "$PID" ]] && kill "$PID" 2>/dev/null || true
  # Share already killed by this point in the success path.
  wait "$SHARE_PID" 2>/dev/null || true
}
trap cleanup EXIT

ppz_s daemon start >/dev/null
ppz_s daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

ppz_s terminal share cindy -- cat </dev/null >/dev/null 2>&1 &
SHARE_PID=$!
wait_for 50 "ppz_s ls 2>/dev/null | grep -q '^cindy.stdout'"

# Capture share pid from the persistence file (its share_pid field).
SHARE_BINDING_PID=$(grep -oE '"share_pid":[0-9]+' "$HOME_S/agent-bindings.json" | head -1 | cut -d: -f2)
echo "share_pid_pre: $([[ -n "$SHARE_BINDING_PID" ]] && echo yes || echo no)"

# Kill -9 the share. The OS reaps it; the persistence file still
# names it. Next lookup must drop the entry.
kill -9 "$SHARE_PID" 2>/dev/null || true
wait "$SHARE_PID" 2>/dev/null || true

# Trigger a lookup that touches the binding table. `ppz status` from
# the test shell hits the daemon and (with ancestor walk) tries to
# match its ancestor chain against bindings. The dead-pid entry is
# revalidated and dropped lazily.
ppz_s status >/dev/null 2>&1 || true

# After the lookup, the persistence file should no longer carry the
# dead binding. (Impl writes through after sweep.)
if grep -q '"handle":"cindy"' "$HOME_S/agent-bindings.json" 2>/dev/null; then
  echo "stale_binding: still_present"
else
  echo "stale_binding: dropped"
fi
