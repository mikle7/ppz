#!/usr/bin/env bash
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create foo >/dev/null
# 70KiB > 64KiB cap. Daemon must reject with E_PAYLOAD_TOO_LARGE (exit 17)
# without contacting NATS.
#
# `--eof` forces atomic single-message mode. The streaming default
# would chunk base64's 76-char-wrapped output into many sub-cap lines
# and never trigger the size guard.
head -c 71680 /dev/urandom | base64 | head -c 71680 | ppz_a broadcast --eof
