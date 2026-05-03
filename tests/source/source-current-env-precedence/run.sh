#!/usr/bin/env bash
# `current` resolution precedence:
#   1. PPZ_CURRENT_HANDLE env var, if set + non-empty
#   2. daemon's current.json[session], otherwise
#
# When env wins, status annotates the value with "(PPZ_CURRENT_HANDLE)"
# so users know where their effective binding is coming from. Critical
# inside `terminal share` (where wrap exports the env var) and useful
# for direnv-style per-directory overrides outside.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
# Bare `source create foo` sets daemon current[session]=foo as a side
# effect (today's behaviour, kept in the refactor).
ppz_a source create foo >/dev/null
# Also create bar so broadcast targeting it succeeds.
ppz_a source create bar >/dev/null
# source create bar made bar the daemon current — switch back to foo so
# the daemon-side state we're testing is foo, not bar.
ppz_a source switch foo >/dev/null

echo "--- env unset: daemon's current wins ---"
ppz_a status | grep '^current source:'

echo "--- env set: env wins, annotated ---"
# Status prints two `current source:` lines + a `warning:` line when env
# and daemon disagree. Normalise the daemon's reported current.json
# path so the test doesn't depend on container layout.
PPZ_CURRENT_HANDLE=bar ppz_a status \
  | grep -E '^(current source:|warning:)' \
  | sed -E 's|/[^ ,)]*/current\.json|PPZ_HOME/current.json|'

echo "--- env set: broadcast targets bar, not foo ---"
PPZ_CURRENT_HANDLE=bar ppz_a broadcast -m "to-bar" >/dev/null
wait_for 20 "ppz_a ls | grep -q to-bar" >/dev/null
ppz_a ls | grep '^bar\.broadcast' | ls_normalize

echo "--- env unset again: falls back to daemon's foo ---"
ppz_a status | grep '^current source:'

echo "--- env unset: broadcast targets foo ---"
ppz_a broadcast -m "to-foo" >/dev/null
wait_for 20 "ppz_a ls | grep -q to-foo" >/dev/null
ppz_a ls | grep '^foo\.broadcast' | ls_normalize
