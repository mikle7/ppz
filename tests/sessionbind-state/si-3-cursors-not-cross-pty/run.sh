#!/usr/bin/env bash
# SI-3 (negative): cursors in different ptys don't share. Cindy's pty
# reads cindy.inbox → advances her cursor. Bob's pty reading
# cindy.inbox would still see unread (cindy and bob are different
# session keys → different cursor state).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a unset namespace >/dev/null

# Create cindy + bob as message-kind sources up front so bare
# `terminal share --` can layer the pty-style pipes on top without
# conflicting on source kind.
ppz_a source create cindy >/dev/null
ppz_a send --from pubsub cindy "msg-for-isolation" >/dev/null
ppz_a set handle bob >/dev/null  # so the source create below works without unset
ppz_a unset handle >/dev/null
ppz_a source create bob >/dev/null
ppz_a unset handle >/dev/null

# Cindy's pty drains her inbox first. Bare share against the current
# handle — set it just for this scope.
ppz_a set handle cindy >/dev/null
PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" \
  ppz terminal share -- sh -c '
    env -u PPZ_CURRENT_HANDLE -u PPZ_SESSION ppz read inbox --bare 2>/dev/null > /tmp/si-3-cindy.txt
  ' </dev/null >/dev/null 2>&1
echo "cindy_pty: $(cat /tmp/si-3-cindy.txt)"
ppz_a unset handle >/dev/null

# Bob's pty reads cindy.inbox explicitly. With per-session cursors,
# bob's session has its own (unset) cursor for cindy.inbox → sees
# the message as new.
ppz_a set handle bob >/dev/null
PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" \
  ppz terminal share -- sh -c '
    env -u PPZ_CURRENT_HANDLE -u PPZ_SESSION ppz read cindy.inbox --bare 2>/dev/null > /tmp/si-3-bob.txt
  ' </dev/null >/dev/null 2>&1
echo "bob_pty: $(cat /tmp/si-3-bob.txt)"

rm -f /tmp/si-3-cindy.txt /tmp/si-3-bob.txt
