#!/usr/bin/env bash
# `ppz send --request-ack` requires a non-empty current source on the
# sender side — if there's no current, the recipient's daemon would have
# nowhere to send the ack. CLI preflights against IPCStatus and exits
# with E_NO_CURRENT_SOURCE before the broadcast IPC call.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
# DELIBERATELY no `source create` — the session has no current source.

# Capture stderr to verify the error code surfaces.
ppz_a send chat "hi" --request-ack 2>&1 | grep -oE '^error: E_[A-Z_]+' || true
