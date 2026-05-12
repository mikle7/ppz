#!/usr/bin/env bash
# RED: `ppz source destroy` must surface as an unknown top-level command.
# Replacement: ppz pipe destroy --recursive HANDLE (locked decision #21).
. /tests/lib/common.sh

stderr=$(ppz_a source destroy cindy 2>&1 1>/dev/null)
rc=$?

echo "exit=$rc"
if echo "$stderr" | grep -qE 'unknown (command|subcommand)'; then
  echo "stderr_contains_unknown=yes"
else
  echo "stderr_contains_unknown=no"
fi
