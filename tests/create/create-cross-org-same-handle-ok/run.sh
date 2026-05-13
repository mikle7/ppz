#!/usr/bin/env bash
. /tests/lib/common.sh
# daemon-a logs into org alpha, daemon-b logs into org beta. The same handle
# 'shared' must be creatable in both orgs (uniqueness is per-org).
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_b daemon login "$PPZ_SERVER_URL" -apikey "$(key_beta)"  >/dev/null
ppz_a source create shared
ppz_b source create shared
