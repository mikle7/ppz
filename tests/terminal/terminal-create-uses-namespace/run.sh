#!/usr/bin/env bash
# Phase 1.5.2: `ppz terminal create H` provisions a pty-kind source at
# the session's current namespace, mirroring `ppz source create`. The
# code already passes Session to IPCCreate so the daemon stamps
# manifold correctly — this test locks the behaviour in.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a set namespace pixel >/dev/null
ppz_a terminal create cindy >/dev/null

# All four auto-pipes should land at pixel.cindy.{inbox,stdctrl,stdin,stdout}.
ppz_a ls | awk '$1 ~ /cindy\./ {print $1}' | sort
