#!/usr/bin/env bash
# A glob/pattern subscription is a lens, not a stored pipe: `subs add
# 'room-%.inbox'` surfaces every matching pipe (room-a.inbox, room-b.inbox)
# and NOT a spurious literal 'room-%.inbox' pipe. The non-matching 'other'
# source is excluded. Same %→* glob the ls --watch matcher already uses.
#
# The pattern renders as a PARENT row with its matched pipes as indented
# children, so attribution is visible (which pattern surfaced which pipe)
# rather than the matches appearing as indistinguishable flat rows.
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
PPZ_SESSION=mysh ppz_a subs ls | ls_normalize
