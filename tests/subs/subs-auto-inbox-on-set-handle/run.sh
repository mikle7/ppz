#!/usr/bin/env bash
# `ppz set handle H` makes the session operate AS H (sets its current
# handle). Like `source create`, that session should then see H's inbox in
# its subs — a plain `ppz subs ls/wait` after `set handle` just works.
#
# james is created from a SEPARATE session so its create-time auto-sub
# lands elsewhere; this scenario proves `set handle` itself subscribes the
# adopting session.
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
PPZ_SESSION=owner ppz_a source create james >/dev/null
PPZ_SESSION=mysh  ppz_a set handle james >/dev/null
PPZ_SESSION=mysh  ppz_a subs ls | ls_normalize | awk '{print $1}'
