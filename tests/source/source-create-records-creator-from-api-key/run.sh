#!/usr/bin/env bash
# A source created via an API key inherits the user attribution stored on
# that key. Seeded keys: alpha-primary→foo, alpha-secondary→bar (both in
# the alpha org). Daemon A runs as foo, daemon B as bar — each creates a
# source, and a list from either daemon shows both with their respective
# creators.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)"  >/dev/null
ppz_b daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha2)" >/dev/null
ppz_a source create chat-foo >/dev/null
ppz_b source create chat-bar >/dev/null

# `--json` carries the per-row username on every (source, pipe) row.
# Both daemons should see both sources (same org, alpha) — assert from
# A so we also confirm cross-daemon visibility.
ppz_a ls --json | jq -c '{handle, pipe, creator}'
