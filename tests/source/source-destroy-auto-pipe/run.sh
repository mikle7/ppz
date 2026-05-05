#!/usr/bin/env bash
# Destroying auto-provisioned pipes (broadcast, inbox) by dotted pattern
# must succeed — they are JetStream-only (not in the pipes table) but are
# visible in ppz ls and should be destroyable without E_PIPE_NOT_FOUND.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create apple >/dev/null

echo "--- before destroy ---"
ppz_a ls | grep '^apple\.' | ls_normalize

ppz_a source destroy 'apple.broadcast'
ppz_a source destroy 'apple.inbox'

echo "--- after destroy ---"
ppz_a ls | grep '^apple\.' | ls_normalize
