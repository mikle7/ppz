#!/usr/bin/env bash
# Reserved pipe names (system, db, inbox) are rejected with E_INVALID_PIPE
# (exit 20). broadcast / stdin / stdout are auto-provisioned by the system
# and aren't user-creatable in this phase either, but their reservation is
# tested separately by the auto-provisioning paths.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a pipe create system
