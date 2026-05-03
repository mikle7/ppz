#!/usr/bin/env bash
# `ppz version` does NOT require login. Pure local lookup of the binary's
# embedded build info — same shape regardless of credential state.
. /tests/lib/common.sh
ppz_a version
