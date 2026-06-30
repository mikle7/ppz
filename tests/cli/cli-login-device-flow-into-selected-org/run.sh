#!/usr/bin/env bash
# End-to-end multi-org repro. bar belongs to alpha + beta and OWNS NEITHER.
# Before the fix, an OAuth login dropped bar onto the server's owner-only
# default org — which for bar is none, so login failed outright; and even
# when a session did list a second org's pipes, the NATS creds were minted
# in the wrong account so reads/sends came back empty.
#
# Now bar logs in via the device flow, selects beta, and the whole chain
# (verify -> token -> /auth/exchange -> NATS JWT) lands in beta: the
# session is bound to beta AND a pipe created there can be written and read
# back over NATS.
. /tests/lib/common.sh

# 1. Launch the real `ppz login` (device flow) in the background.
LOG=$(mktemp)
ppz_a login "$PPZ_SERVER_URL" > "$LOG" 2>&1 &
CLI_PID=$!

# 2. Parse the printed user_code.
USER_CODE=""
for i in $(seq 1 30); do
  USER_CODE=$(grep -oE '[A-Z0-9]{4}-[A-Z0-9]{4}' "$LOG" | head -1)
  [[ -n "$USER_CODE" ]] && break
  sleep 0.5
done
echo "user_code_extracted: $([[ -n "$USER_CODE" ]] && echo true || echo false)"

# 3. As bar, read the verify page, pull beta's org id from the dropdown,
#    and approve the session into beta.
COOKIE_JAR=$(mktemp)
trap "rm -f $COOKIE_JAR $LOG" EXIT
curl_server "/dev/login?user=bar" -X POST -c "$COOKIE_JAR" -o /dev/null -s
PAGE=$(curl_server "/oauth/device/verify?user_code=$USER_CODE" -b "$COOKIE_JAR" -s)
BETA_ID=$(echo "$PAGE" | grep -oE '<option value="[0-9a-f-]+"[^>]*>beta</option>' | grep -oE '[0-9a-f-]{36}' | head -1)
echo "beta_id_found: $([[ -n "$BETA_ID" ]] && echo true || echo false)"
curl_server "/oauth/device/verify" -X POST -b "$COOKIE_JAR" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    --data-urlencode "user_code=$USER_CODE" \
    --data-urlencode "account_id=$BETA_ID" \
    --max-redirs 0 -o /dev/null -s

# 4. Wait for the CLI to detect approval + exit clean.
wait "$CLI_PID"
echo "cli_exit_code: $?"

# 5. The session is bound to the SELECTED org, not a server default.
ppz_a status | grep '^account:'

# 6. The repro: a pipe created in beta can be written and read back over
#    NATS — the operations that came back empty under the bug.
echo "--- send + read back from selected org ---"
ppz_a source create box >/dev/null
ppz_a send box "hello-from-beta" >/dev/null
wait_for 30 "ppz_a reread box.inbox -l 1 --json | grep -q hello-from-beta" >/dev/null
echo "payload: $(ppz_a reread box.inbox -l 1 --json | jq -r '.payload')"
