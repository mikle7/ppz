#!/usr/bin/env bash
# RED — docs/specs/schedule.md. `ppz schedule ls` renders the agreed
# table (ID NAMESPACE PIPE SCHEDULE NEXT LAST PAYLOAD CREATOR) with
# the `ppz ls` conventions: `-` for missing, relative NEXT/LAST by
# default, --iso for RFC3339, --json as JSONL with full payload and
# null last_at. Rows sort by NEXT ascending (the --every 1h row fires
# sooner than the year-2999 one-off, so it lists first).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a pipe create room >/dev/null
ppz_a source create alice >/dev/null

ppz_a send room "tick tock" --every 1h >/dev/null 2>&1
ppz_a send alice "one-off standup" --at "2999-01-02T03:04:05Z" >/dev/null 2>&1

# Normalise for diffing: drop the volatile ID column, collapse padding,
# RELATIVE-ise the future-relative NEXT cell. (RFC3339 cells are handled
# by the harness normalizer.)
sched_normalize() {
  awk '{ $1 = ""; sub(/^[ \t]+/, ""); print }' \
    | sed -E 's/[[:space:]]+/ /g' \
    | sed -E 's/in [0-9]+ (seconds?|minutes?|hours?|days?)/RELATIVE/'
}

echo "--- table ---"
ppz_a schedule ls | sched_normalize
echo "--- iso ---"
ppz_a schedule ls --iso | sched_normalize
echo "--- json ---"
ppz_a schedule ls --json | grep -c '"id":"'
ppz_a schedule ls --json | grep -o '"schedule":"every"'
ppz_a schedule ls --json | grep -o '"schedule":"at"'
ppz_a schedule ls --json | grep -o '"last_at":null' | sort -u
