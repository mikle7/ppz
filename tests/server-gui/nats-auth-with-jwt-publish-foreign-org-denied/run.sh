#!/usr/bin/env bash
# Phase 3.5 cross-tenant data plane isolation: a publish from alpha
# targeting a beta-shaped subject does NOT reach beta.
#
# Pre-3.5 (subject-prefix isolation): alpha's user JWT had pub allow
# only `<alpha-org-id>.>`. Publishing to `<beta-org-id>.>` was
# rejected at the NATS auth layer.
#
# Post-3.5 (per-org accounts): alpha's user JWT has broad pub allow
# inside the tenant-alpha account. Alpha can publish a literal subject
# string `<beta-org-id>.foo.broadcast` — but it lands in tenant-alpha's
# subject namespace. Tenant-beta has its own NATS account; nobody in
# beta is subscribed to alpha's namespace; the message goes nowhere.
#
# Test verifies the security property (beta cannot receive a forged
# alpha-originated publish) through the receiving side. The mechanism
# is account-boundary containment, not auth-layer denial.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_b daemon login "$PPZ_SERVER_URL" -apikey "$(key_beta)" >/dev/null

JWT_A=$(jq -r '.nats_user_jwt' "${PPZ_DAEMON_A_HOME:-/tmp/a}/credentials")
SEED_A=$(jq -r '.nats_user_seed' "${PPZ_DAEMON_A_HOME:-/tmp/a}/credentials")
JWT_B=$(jq -r '.nats_user_jwt' "${PPZ_DAEMON_B_HOME:-/tmp/b}/credentials")
SEED_B=$(jq -r '.nats_user_seed' "${PPZ_DAEMON_B_HOME:-/tmp/b}/credentials")
BETA_ORG=$(jq -r '.org_id' "${PPZ_DAEMON_B_HOME:-/tmp/b}/credentials")

CREDS_A=$(mktemp)
CREDS_B=$(mktemp)
trap "rm -f $CREDS_A $CREDS_B" EXIT
cat > "$CREDS_A" <<EOF
-----BEGIN NATS USER JWT-----
$JWT_A
------END NATS USER JWT------

-----BEGIN USER NKEY SEED-----
$SEED_A
------END USER NKEY SEED------
EOF
cat > "$CREDS_B" <<EOF
-----BEGIN NATS USER JWT-----
$JWT_B
------END NATS USER JWT------

-----BEGIN USER NKEY SEED-----
$SEED_B
------END USER NKEY SEED------
EOF

echo "--- beta subscribes to its own subjects (no forged messages allowed in) ---"
# Run the sub in the background, give it a moment to attach.
RECV=$(mktemp)
trap "rm -f $RECV" EXIT
( nats --server="nats://ppz-server:4222" --creds="$CREDS_B" \
    sub "${BETA_ORG}.>" --count=1 --timeout=1s > "$RECV" 2>&1 ) &
SUB_PID=$!
sleep 0.3

echo ""
echo "--- alpha publishes a beta-shaped subject ---"
nats --server="nats://ppz-server:4222" --creds="$CREDS_A" \
    pub "${BETA_ORG}.intruder.broadcast" "from-alpha" --timeout=3s 2>&1 \
    | grep -oE 'Published [0-9]+ bytes' | head -1
echo "(message published into alpha's account namespace; beta cannot see it)"

echo ""
echo "--- beta's subscriber timed out without receiving the forged message ---"
# Watchdog: kill the subscriber after 2s if `nats sub --timeout=1s` failed
# to self-terminate. Under per-org account auth the CLI's own timeout has
# been observed to hang indefinitely (CI run 25245101413 ran 1h56m before
# being cancelled here), so we don't rely on it.
( sleep 2; kill -TERM "$SUB_PID" 2>/dev/null ) &
KILLER=$!
wait "$SUB_PID" 2>/dev/null || true
kill -TERM "$KILLER" 2>/dev/null || true
wait "$KILLER" 2>/dev/null || true
if grep -q "from-alpha" "$RECV"; then
  echo "beta_received_forged_message: yes"
else
  echo "beta_received_forged_message: no"
fi
