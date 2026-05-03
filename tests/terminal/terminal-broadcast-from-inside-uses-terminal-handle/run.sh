#!/usr/bin/env bash
# Inside the wrapped child, `ppz broadcast` must publish to the *terminal's*
# handle, not the daemon's "current" handle. Implemented by the terminal
# process exporting PPZ_CURRENT_HANDLE=<handle> into the child's env;
# cmdBroadcast honors it as an override.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create other >/dev/null            # daemon's current = "other"
ppz_a terminal share term-pipe -- sh -c 'ppz broadcast -m "from-inside"' >/dev/null

# Confirm the broadcast went to term-pipe.broadcast, not other.broadcast.
ppz_a read term-pipe.broadcast
echo "---other---"
ppz_a read other.broadcast
