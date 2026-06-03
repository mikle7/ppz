#!/usr/bin/env bash
# From a personal shell (PPZ_SESSION=desk), alice.inbox is NOT self
# (session "desk" != handle "alice"). Removing it is allowed, exit 0.
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create alice >/dev/null
export PPZ_SESSION=desk
ppz_a subs add alice.inbox >/dev/null
echo "--- after add ---"
ppz_a subs ls | ls_normalize | awk '{print $1}'
ppz_a subs rm alice.inbox; echo "rm-rc=$?"
echo "--- after rm ---"
ppz_a subs ls | ls_normalize | awk '{print $1}'
