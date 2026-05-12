#!/usr/bin/env bash
# Phase 3 happy-path: a daemon authenticated via the device flow gets
# a User JWT through /auth/exchange and uses it to connect to NATS.
# Publishing within its own org succeeds.
#
# This is the existing login → send flow, but post-Phase-3 the
# daemon's NATS connection MUST present a JWT. Failure mode pre-fix:
# /auth/exchange returns no JWT, daemon falls back to plain
# nats.Connect, server now rejects with Authorization Violation.
. /tests/lib/common.sh

# Login as foo via the dev login + device flow.
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

echo "--- daemon status reports a NATS user JWT in credentials ---"
# After Phase 3 the daemon's credentials file should include nats_jwt
# alongside the bearer token. We read it via `ppz status --json` once
# that surface lands; for RED, we grep the credentials file directly.
if [[ -f "${PPZ_DAEMON_A_HOME:-/tmp/a}/credentials" ]]; then
  grep -q 'nats_user_jwt' "${PPZ_DAEMON_A_HOME:-/tmp/a}/credentials" \
    && echo "nats_jwt_in_credentials: true" \
    || echo "nats_jwt_in_credentials: false"
else
  echo "credentials_file_missing"
fi

echo ""
echo "--- send within own org succeeds ---"
ppz_a terminal create chat >/dev/null
ppz_a send chat.inbox "phase 3 hello" >/dev/null && echo "send=ok" || echo "send=fail"

echo ""
echo "--- ls sees the message ---"
wait_for 20 "ppz_a ls | grep -q 'phase 3 hello'" >/dev/null \
  && echo "ls=ok" \
  || echo "ls=fail"
