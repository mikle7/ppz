#!/usr/bin/env bash
# add the same room twice → one entry. rm it → gone. rm again (absent) →
# no-op, exit 0. A dotless target stays an uncollared pipe (read-style),
# stored verbatim as "room" — no .inbox sugar.
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
export PPZ_SESSION=mysh
ppz_a subs add room >/dev/null
ppz_a subs add room >/dev/null
echo "--- after add x2 ---"
ppz_a subs ls | ls_normalize | awk '{print $1}'
ppz_a subs rm room >/dev/null
ppz_a subs rm room; echo "rm-absent-rc=$?"
echo "--- after rm ---"
ppz_a subs ls | ls_normalize | awk '{print $1}'
