#!/usr/bin/env bash
# A session with no subscriptions prints nothing (exit 0) — same as
# `ppz ls` on an empty org. The subscription list is the source of
# truth; empty list, empty output.
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
PPZ_SESSION=fresh ppz_a subs ls
