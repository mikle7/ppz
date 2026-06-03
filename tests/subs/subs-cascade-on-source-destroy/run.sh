#!/usr/bin/env bash
# A personal shell subscribes to alice.inbox. Destroying the alice source
# sweeps that sub out of the shell's subs file — no zombie subs surviving
# a destroy/recreate. Mirrors the #88 cursor sweep.
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create alice >/dev/null
export PPZ_SESSION=desk
ppz_a subs add alice.inbox >/dev/null
echo "--- before destroy ---"
ppz_a subs ls | ls_normalize | awk '{print $1}'
ppz_a source destroy alice >/dev/null
echo "--- after destroy ---"
ppz_a subs ls | ls_normalize | awk '{print $1}'
