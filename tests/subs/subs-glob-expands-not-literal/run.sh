#!/usr/bin/env bash
# A glob/pattern subscription is a lens, not a stored row: `subs add
# 'room-%.inbox'` surfaces every matching pipe in subs ls (room-a.inbox,
# room-b.inbox) and NOT a spurious literal 'room-%.inbox' row. The
# non-matching 'other' source is excluded. Same %→* glob the ls --watch
# matcher already uses (reused, not modified).
#
# The sources are created from a SEPARATE session ('setup') so their
# auto-subscribed inboxes don't land in mysh — this scenario tests glob
# expansion of an explicit mysh sub, not the source-create auto-sub.
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
PPZ_SESSION=setup ppz_a source create room-a >/dev/null
PPZ_SESSION=setup ppz_a source create room-b >/dev/null
PPZ_SESSION=setup ppz_a source create other  >/dev/null
PPZ_SESSION=mysh ppz_a subs add 'room-%.inbox' >/dev/null
PPZ_SESSION=mysh ppz_a subs ls | ls_normalize | awk '{print $1}' | sort
