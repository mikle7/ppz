#!/usr/bin/env bash
# `ppz ls <pattern>` matches the full <handle>.<pipe> target, not the pipe
# segment alone. So `ls room` hits only the uncollared `room`, and you use
# `ls '*.room'` to reach a collared <handle>.room. (Shell parallel: `ls Mus`
# vs `ls Mus*`.)
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create alice >/dev/null
ppz_a pipe create alice.room >/dev/null
ppz_a unset handle >/dev/null 2>&1
ppz_a pipe create room >/dev/null
echo "--- ls room (full-name: uncollared room only) ---"
ppz_a ls room | ls_normalize | awk '{print $1}' | sort
echo "--- ls '*.room' (glob: collared alice.room) ---"
ppz_a ls '*.room' | ls_normalize | awk '{print $1}' | sort
