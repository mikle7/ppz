#!/usr/bin/env bash
# A dotless sub `room` is the uncollared pipe `room` only — full-name
# matching means it must NOT surface a collared <handle>.room. (Review
# finding #2, fixed by the matchAnyTarget full-name change.)
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
export PPZ_SESSION=mysh
ppz_a source create alice >/dev/null
ppz_a pipe create alice.room >/dev/null
ppz_a subs add room >/dev/null
ppz_a subs ls | ls_normalize | awk '{print $1}' | sort
