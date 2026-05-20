#!/usr/bin/env bash
# RC-C: persistence file corruption → loader returns empty (clean,
# no crash) → share's next IPC re-registers via E_BINDING_UNKNOWN.
#
# Window of brokenness: until the share emits any IPC (e.g.
# publishing wrapped-pty stdout). We force a stdout publish to
# accelerate.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

HOME_S=/tmp/rc-c
rm -rf "$HOME_S"; mkdir -p "$HOME_S"
SOCK_S=$HOME_S/daemon.sock

ppz_s() { PPZ_HOME=$HOME_S PPZ_IPC_SOCKET=$SOCK_S ppz "$@"; }

cleanup() {
  PID=$(cat "$HOME_S/daemon.pid" 2>/dev/null || true)
  [[ -n "$PID" ]] && kill "$PID" 2>/dev/null || true
  kill "$SHARE_PID" 2>/dev/null || true
  wait "$SHARE_PID" 2>/dev/null || true
}
trap cleanup EXIT

ppz_s daemon start >/dev/null
ppz_s daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

# stty -echo + cat so stdin echoes don't fight with us.
ppz_s terminal share cindy -- bash -c 'stty -icanon -echo 2>/dev/null; cat' </dev/null >/dev/null 2>&1 &
SHARE_PID=$!
wait_for 50 "ppz_s ls 2>/dev/null | grep -q '^cindy.stdout'"

# Stop daemon, corrupt the persistence file, restart.
ppz_s daemon stop >/dev/null
echo "GARBAGE_NOT_JSON" > "$HOME_S/agent-bindings.json"
ppz_s daemon start >/dev/null
ppz_s daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

# Drive an IPC from inside the share by piping a byte to cindy.stdin
# (which forwards through the daemon → pty → cat → publish to
# cindy.stdout). That stdout publish is THE share's IPC; the daemon
# detects no binding for cindy and replies E_BINDING_UNKNOWN; share
# re-registers; binding restored.
ppz_a send cindy.stdin "x" >/dev/null
sleep 1

# Verify the binding is back: in-pty status would do it but we can't
# easily inject one. Instead, check the persistence file is now
# valid JSON with a cindy entry.
if grep -q '"handle":"cindy"' "$HOME_S/agent-bindings.json"; then
  echo "relearned: yes"
else
  echo "relearned: no ($(head -c 80 "$HOME_S/agent-bindings.json"))"
fi
