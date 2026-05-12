#!/usr/bin/env bash
# A user-created pipe carries its own creator, distinct from the
# source's creator. foo (alpha-primary) creates the source `chat`;
# bar (alpha-secondary) creates a custom pipe `notes` on it. Auto-
# pipes inherit the source creator (foo); the custom pipe carries
# bar.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)"  >/dev/null  # foo
ppz_b daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha2)" >/dev/null  # bar

ppz_a terminal create chat       >/dev/null   # source created by foo
ppz_b pipe create chat.notes   >/dev/null   # pipe created by bar on foo's source

# Daemon A's view is built from the server's GET /api/v1/sources +
# JetStream enrichment; allow up to a beat for JetStream stream
# provisioning to complete on the new pipe.
wait_for 20 "ppz_a ls --json | grep -q '\"notes\"'" >/dev/null

ppz_a ls --json | jq -c '{handle, pipe, creator}'
