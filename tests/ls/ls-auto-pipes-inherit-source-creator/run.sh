#!/usr/bin/env bash
# Auto-provisioned pipes (broadcast, inbox) don't have rows in the
# `pipes` table — there's no per-pipe creator to read. The renderer
# falls back to the source's creator for those rows.
#
# foo (alpha-primary) creates a source. bar (alpha-secondary) creates
# a custom pipe `notes` on the same source. The auto-pipes both show
# foo (inherited from source); the custom pipe shows bar (its own
# creator) — proving inheritance is fallback-only, not blanket.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)"  >/dev/null  # foo
ppz_b daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha2)" >/dev/null  # bar

ppz_a terminal create chat     >/dev/null
ppz_b pipe create chat.notes >/dev/null

wait_for 20 "ppz_a ls --json | grep -q '\"notes\"'" >/dev/null

# Project just (pipe, creator) so the diff is tight.
ppz_a ls --json | jq -c '{pipe, creator}'
