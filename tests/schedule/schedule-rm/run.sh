#!/usr/bin/env bash
# RED — docs/specs/schedule.md. `ppz schedule rm <id>` removes a
# schedule by the short id printed at creation; removing an unknown id
# surfaces E_SCHEDULE_NOT_FOUND (offending-name errors precedent).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create alice >/dev/null

out=$(ppz_a send alice "future ping" --at "2999-01-01T00:00:00Z" 2>&1)
sid=$(echo "$out" | grep -oE 'id=[a-f0-9]{8}' | head -1 | cut -d= -f2)
echo "captured-id=$([ -n "$sid" ] && echo yes || echo no)"
echo "rows-before=$(ppz_a schedule ls | grep -c "$sid")"

ppz_a schedule rm "$sid" 2>&1 | sed "s/$sid/SCHID/"
echo "rows-after=$(ppz_a schedule ls | wc -l | tr -d ' ')"

# Second rm on the same id: gone is gone.
code=$(ppz_a schedule rm "$sid" 2>&1 | grep -oE 'E_[A-Z_]+' | head -1)
echo "rm-again=$code"
