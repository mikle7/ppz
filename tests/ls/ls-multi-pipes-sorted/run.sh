#!/usr/bin/env bash
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create gamma >/dev/null
ppz_a terminal create alpha >/dev/null
ppz_a terminal create beta  >/dev/null
# Order of creation is gamma, alpha, beta. ls must sort handles ASC.
ppz_a ls | ls_normalize
