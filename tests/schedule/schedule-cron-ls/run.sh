#!/usr/bin/env bash
# RED — docs/specs/schedule.md. `--cron "<expr>"` creates a wall-clock
# recurring schedule; `schedule ls` renders the SCHEDULE cell as
# `cron <expr> <IANA tz>`. The test containers run with no $TZ, so the
# CLI's device-zone capture falls back to UTC — pinning both the cell
# format and the fallback.
#
# Kept separate from schedule-ls-table: a cron row's NEXT depends on
# the wall-clock day the suite runs, so mixing it with --every rows
# would make the NEXT-ascending sort order nondeterministic.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create alice >/dev/null

ppz_a send alice "weekly cron" --cron "0 10 * * MON" >/dev/null 2>&1

sched_normalize() {
  awk '{ $1 = ""; sub(/^[ \t]+/, ""); print }' \
    | sed -E 's/[[:space:]]+/ /g' \
    | sed -E 's/in [0-9]+ (seconds?|minutes?|hours?|days?)/RELATIVE/'
}

ppz_a schedule ls | sched_normalize
ppz_a schedule ls --json | grep -o '"schedule":"cron"'
ppz_a schedule ls --json | grep -o '"spec":"0 10 \* \* MON"'
ppz_a schedule ls --json | grep -o '"tz":"UTC"'
