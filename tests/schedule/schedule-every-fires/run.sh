#!/usr/bin/env bash
# RED — docs/specs/schedule.md. `--every <dur>` fires repeatedly on the
# interval grid: an --every 1s schedule must produce at least three
# messages on the target pipe. (1s is the pinned minimum interval —
# also what keeps this scenario inside the 30s harness ceiling.)
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a pipe create tick >/dev/null

ppz_a send tick "beat" --every 1s >/dev/null 2>&1
echo "create-exit=$?"

wait_for 200 '[ "$(ppz_a reread tick -l 5 --bare 2>/dev/null | wc -l)" -ge 3 ]'
echo "three-beats=$?"
ppz_a reread tick -l 1 --bare
