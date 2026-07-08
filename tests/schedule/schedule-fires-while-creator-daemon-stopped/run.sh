#!/usr/bin/env bash
# RED — docs/specs/schedule.md. THE durability pin: the schedule lives
# on the server, so it fires even when the daemon that created it is
# dead. An ad-hoc daemon K registers a one-off and is stopped before
# the fire instant; daemon A (a different process, same org via the
# alpha-secondary key) then observes the message — proof the publish
# came from the server-side scheduler, not any client.
. /tests/lib/common.sh

HOME_K=/tmp/k-sched
rm -rf "$HOME_K"; mkdir -p "$HOME_K"
SOCK_K=$HOME_K/daemon.sock
ppz_k() { PPZ_HOME=$HOME_K PPZ_IPC_SOCKET=$SOCK_K ppz "$@"; }
cleanup() {
  PID=$(cat "$HOME_K/daemon.pid" 2>/dev/null || true)
  [[ -n "$PID" ]] && kill "$PID" 2>/dev/null || true
}
trap cleanup EXIT

ppz_k daemon start >/dev/null
ppz_k daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_k source create kbot >/dev/null

ppz_k send kbot "durable ping" --at +3s >/dev/null 2>&1
echo "scheduled-exit=$?"

ppz_k daemon stop >/dev/null 2>&1
echo "daemon-k=stopped"

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha2)" >/dev/null
wait_for 100 'ppz_a reread kbot.inbox -l 1 --bare 2>/dev/null | grep -q "durable ping"'
echo "fired-while-stopped=$?"
ppz_a reread kbot.inbox -l 1 --bare
