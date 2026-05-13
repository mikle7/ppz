#!/usr/bin/env bash
. /tests/lib/common.sh
# No login. create must fail with E_NOT_LOGGED_IN (exit 10) and write nothing
# to stdout.
ppz_a source create foo
