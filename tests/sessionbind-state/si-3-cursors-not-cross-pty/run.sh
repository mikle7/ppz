#!/usr/bin/env bash
# SI-3 (negative): cursors in different ptys don't share. Cindy's pty
# reads cindy.inbox → advances her cursor. Bob's pty reading
# cindy.inbox would still see unread (cindy and bob are different
# session keys → different cursor state).
#
# Note: bob doesn't normally read cindy.inbox in production. This
# test exercises the isolation invariant via a contrived path.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a unset namespace >/dev/null

# Source for the message.
ppz_a source create cindy >/dev/null
ppz_a send --from pubsub cindy "msg-for-isolation" >/dev/null
ppz_a unset handle >/dev/null

# Cindy's pty drains her inbox first.
PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" \
  ppz terminal share cindy -- sh -c '
    ppz read inbox --bare 2>/dev/null > /tmp/si-3-cindy.txt
  ' </dev/null >/dev/null 2>&1
echo "cindy_pty: $(cat /tmp/si-3-cindy.txt)"

# Bob's pty reads cindy.inbox explicitly. With per-session cursors,
# bob's session has its own (unset) cursor for cindy.inbox → sees
# the message as new.
PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" \
  ppz terminal share bob -- sh -c '
    ppz read cindy.inbox --bare 2>/dev/null > /tmp/si-3-bob.txt
  ' </dev/null >/dev/null 2>&1
echo "bob_pty: $(cat /tmp/si-3-bob.txt)"

rm -f /tmp/si-3-cindy.txt /tmp/si-3-bob.txt
