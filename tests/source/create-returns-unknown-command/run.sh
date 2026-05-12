#!/usr/bin/env bash
# RED: `ppz source create` must surface as an unknown top-level command
# after Phase 1 cycle 3 strips the source verb. Replacement flows:
#   ppz source create HANDLE  →  ppz terminal create HANDLE
#                                  ppz agent create HANDLE
# See OSS-PIPESCLOUD-ARCHITECTURE-SPLIT (private) locked decisions
# #18 and #21.
. /tests/lib/common.sh

stderr=$(ppz_a source create cindy 2>&1 1>/dev/null)
rc=$?

echo "exit=$rc"
if echo "$stderr" | grep -qE 'unknown (command|subcommand)'; then
  echo "stderr_contains_unknown=yes"
else
  echo "stderr_contains_unknown=no"
fi
