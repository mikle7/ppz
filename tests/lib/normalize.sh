#!/usr/bin/env bash
# Read stdin, replace volatile tokens with stable placeholders, write stdout.
#
# Replacements (applied in order):
#   pid=<digits>            -> pid=PID
#   key=<8 lowercase alnum> -> key=KEYPREFIX
#   <UUID>                  -> UUID            (any v1..v8 hyphenated UUID)
#   <RFC3339>               -> TIMESTAMP       (e.g. 2026-04-25T12:34:56Z or with offset)
#   <ORG_ID seeded alpha>   -> ORG_ALPHA       (looked up from /seed/org-alpha.txt if present)
#   <ORG_ID seeded beta>    -> ORG_BETA        (looked up from /seed/org-beta.txt if present)
#
# The seeded org IDs are produced by the server seeder into /seed/org-*.txt
# at compose-up time. If the files don't exist (e.g. running outside compose),
# org IDs are left as raw UUIDs which then collapse to UUID via the next rule —
# tests that depend on org-name disambiguation MUST run inside compose.

set -u
set -o pipefail

org_alpha_id="$(cat /seed/org-alpha.txt 2>/dev/null || echo)"
org_beta_id="$(cat /seed/org-beta.txt  2>/dev/null || echo)"

sed_args=(
  -E
  -e 's/pid=[0-9]+/pid=PID/g'
  -e 's/key=[a-z0-9]{8}/key=KEYPREFIX/g'
)

if [[ -n "$org_alpha_id" ]]; then
  sed_args+=(-e "s/${org_alpha_id}/ORG_ALPHA/g")
fi
if [[ -n "$org_beta_id" ]]; then
  sed_args+=(-e "s/${org_beta_id}/ORG_BETA/g")
fi

# UUID pattern (any version, lowercase hex).
sed_args+=(-e 's/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/UUID/g')

# RFC3339 timestamp (Z or +HH:MM offset, optional fractional seconds).
sed_args+=(-e 's/[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})/TIMESTAMP/g')

# `ppz version` output: "ppz <version> (<sha>)" — normalize to a stable token
# so dev / tagged / dirty builds all diff against the same expected.txt.
sed_args+=(-e 's/^ppz [^ ]+ \([^)]+\)$/ppz VERSION (SHA)/')

exec sed "${sed_args[@]}"
