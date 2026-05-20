#!/usr/bin/env bash
# AS-1 (fail-closed): a bare shell with no current handle set and no
# pty binding sends to david → daemon refuses with
# E_NO_CURRENT_SOURCE. Today (pre-Layer-2), this succeeds with empty
# sender (the existing
# tests/send/send-uncollared-stamps-empty-without-handle/ fixture
# encodes that broken behavior). After Layer 2, it must fail.
#
# This is the negative companion to as-1-after-fix-stamps-sender: an
# unbound caller cannot publish anonymously regardless of how they
# got there.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a unset namespace >/dev/null
ppz_a source create david >/dev/null
ppz_a unset handle >/dev/null

# Direct send, no pty, no binding, no current. Capture stderr (where
# the error code lands per the standard CLI shape) and the exit code.
err=$(mktemp)
ppz_a send david "should-be-rejected" 2>"$err"
rc=$?

# Look for ENoCurrentSource in stderr; report what we found.
if grep -qE "E_NO_CURRENT_SOURCE|no current source" "$err"; then
  echo "rejected: E_NO_CURRENT_SOURCE"
else
  echo "rejected: unexpected ($(head -1 "$err"))"
fi
echo "exit_code: $rc"

# Verify no message landed in david's inbox.
count=$(ppz_a reread david.inbox 2>/dev/null | grep -c "should-be-rejected" || echo 0)
echo "david_inbox_count: $count"

rm -f "$err"
