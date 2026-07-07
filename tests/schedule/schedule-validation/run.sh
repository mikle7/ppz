#!/usr/bin/env bash
# RED — docs/specs/schedule.md. Client-side validation: the schedule
# flags are mutually exclusive (and incompatible with --request-ack),
# specs must parse, and --at must be strictly future. All rejections
# happen before any IPC/server call — `schedule ls` stays empty.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create alice >/dev/null

try() {
  local label=$1; shift
  local out
  if out=$(ppz_a send alice "x" "$@" 2>&1); then
    echo "$label=accepted"
  else
    echo "$label=rejected"
  fi
  echo "$out" | grep -oE '(mutually exclusive|invalid --every|invalid --cron|--at is in the past|cannot combine --request-ack)' | head -1
}

try at-plus-every --at +1m --every 5m
try at-plus-cron --at +1m --cron "0 10 * * MON"
try bad-every --every nonsense
try sub-second-every --every 500ms
try bad-cron --cron "not a cron"
try past-at --at "2000-01-01T00:00:00Z"
try ack-with-at --request-ack --at +1m

echo "rows=$(ppz_a schedule ls | wc -l | tr -d ' ')"
