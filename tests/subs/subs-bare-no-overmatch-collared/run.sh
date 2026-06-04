#!/usr/bin/env bash
# A dotless sub `room` is the uncollared pipe `room` only — full-name
# matching means it must NOT surface a collared <handle>.room. (Review
# finding #2, fixed by the matchAnyTarget full-name change.)
#
# alice is created from a SEPARATE session so its auto-subscribed inbox
# (source create now subscribes the creating session) doesn't land in mysh
# — this scenario tests collared-vs-uncollared matching, not the auto-sub.
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
PPZ_SESSION=setup ppz_a source create alice >/dev/null
PPZ_SESSION=setup ppz_a pipe create alice.room >/dev/null
PPZ_SESSION=mysh ppz_a subs add room >/dev/null
PPZ_SESSION=mysh ppz_a subs ls | ls_normalize | awk '{print $1}' | sort
