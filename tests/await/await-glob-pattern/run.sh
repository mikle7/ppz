#!/usr/bin/env bash
# Glob patterns: `ppz await "*room"` wakes only on pipes whose name (or
# manifold.pipe path) matches the glob. Traffic to a non-matching pipe
# does not wake the watch; matching traffic does.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1
ppz_a pipe create team-room >/dev/null
ppz_a pipe create plaza >/dev/null

# Non-matching traffic first — should NOT cause an early return.
ppz_a send plaza "noise" >/dev/null
wait_for 20 "ppz_a ls | grep -q noise" >/dev/null
# (plaza has unread but our pattern *room shouldn't match it.)

# Now the matching trigger.
ppz_a send team-room "room hello" >/dev/null
wait_for 20 "ppz_a ls | grep -q 'room hello'" >/dev/null

ppz_a await '*room'
