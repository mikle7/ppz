#!/usr/bin/env bash
# Destroying auto-provisioned pipes (broadcast, inbox) by dotted pattern
# must succeed — they are JetStream-only (not in the pipes table) but are
# visible in ppz ls and should be destroyable without E_PIPE_NOT_FOUND.
# Note: auto-provisioned pipes still appear in ppz ls after stream deletion
# (the daemon lists them by source kind), so we assert on exit code only.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create apple >/dev/null

ppz_a source destroy 'apple.broadcast'
ppz_a source destroy 'apple.inbox'
