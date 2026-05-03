#!/usr/bin/env bash
. /tests/lib/common.sh
# Use an obviously-invalid key. The server must return E_INVALID_API_KEY (exit 12).
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "not-a-real-key-0000"
# Status must still show 'not logged in' (no partial-state stored).
ppz_a status
