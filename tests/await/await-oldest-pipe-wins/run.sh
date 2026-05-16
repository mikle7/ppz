#!/usr/bin/env bash
# Two uncollared pipes both have unread. Tie-break = oldest LastAt.
# We publish to alpha first, sleep, then publish to beta — alpha is
# older. Await drains alpha and leaves beta unread (verifiable via ls).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1
ppz_a pipe create alpha >/dev/null
ppz_a pipe create beta  >/dev/null

ppz_a send alpha "older payload" >/dev/null
sleep 1
ppz_a send beta  "newer payload" >/dev/null
wait_for 20 "ppz_a ls | grep -q 'newer payload'" >/dev/null

ppz_a await alpha beta

# alpha should now be drained (unread=0), beta still unread.
echo "--- ls after await ---"
ppz_a ls | ls_normalize | grep -E '^(alpha|beta) ' | sort
