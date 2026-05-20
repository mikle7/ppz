#!/usr/bin/env bash
# Phase 1.5.1: bare `ppz pipe destroy LEAF` ALWAYS targets uncollared at
# current namespace. Symmetric with create — current_handle never
# affects bare destroy routing.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
# Two pipes with the same leaf at different shapes:
#   foo (source) + foo.room (collared under foo)
#   room (uncollared at root)
ppz_a source create foo >/dev/null
ppz_a pipe create foo.room >/dev/null
ppz_a unset handle >/dev/null
ppz_a pipe create room >/dev/null
ppz_a source create foo2 >/dev/null
# foo2 is now current handle. Bare destroy must target uncollared, not foo2.room.
ppz_a pipe destroy room
ppz_a ls | ls_normalize | grep -E '^(room|foo\.room) ' | cut -d' ' -f1 | sort
