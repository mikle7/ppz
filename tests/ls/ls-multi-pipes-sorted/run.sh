#!/usr/bin/env bash
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create gamma >/dev/null
ppz_a source create alpha >/dev/null
ppz_a source create beta  >/dev/null
# Order of creation is gamma, alpha, beta. ls must sort handles ASC.
ppz_a ls | ls_normalize
