#!/usr/bin/env bash
# Destroying the auto-provisioned inbox pipe by dotted pattern must
# succeed — it's JetStream-only (not in the pipes table) but visible
# in ppz ls and destroyable without E_PIPE_NOT_FOUND. Note: auto-
# provisioned pipes still appear in ppz ls after stream deletion
# (the daemon lists them by source kind), so we assert on exit code
# only.
#
# Post-Phase 1: the broadcast auto-pipe is gone (locked decision #16),
# so inbox is the only auto-pipe to exercise here.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create apple >/dev/null

ppz_a source destroy 'apple.inbox'
