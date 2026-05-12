#!/usr/bin/env bash
. /tests/lib/common.sh
# 'a' is in org alpha, 'b' is in org beta. Each ls must show only its own
# org's pipes — no leakage.
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_b daemon login "$PPZ_SERVER_URL" -apikey "$(key_beta)"  >/dev/null
ppz_a terminal create alpha-only >/dev/null
ppz_b terminal create beta-only  >/dev/null
echo "--- a ---"
ppz_a ls | ls_normalize
echo "--- b ---"
ppz_b ls | ls_normalize
