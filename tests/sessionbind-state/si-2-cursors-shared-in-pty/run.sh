#!/usr/bin/env bash
# SI-2: two subprocesses in the SAME pty share read cursors. First
# subprocess reads cindy.inbox → cursor advances. Second subprocess
# reads cindy.inbox → sees no new messages (cursor already past).
# Both subprocesses resolve to the same session via ancestor walk →
# same cursor.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a unset namespace >/dev/null

# Pre-send a message so cindy.inbox has unread.
ppz_a source create cindy >/dev/null  # ensure source exists; share will use it
ppz_a send --from pubsub cindy "msg-for-cursor-test" >/dev/null
ppz_a unset handle >/dev/null

PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" \
  ppz terminal share cindy -- sh -c '
    # First subprocess: drain new messages.
    echo "sub1:" > /tmp/si-2-cap.txt
    ppz read inbox --bare 2>/dev/null >> /tmp/si-2-cap.txt
    echo "sub2:" >> /tmp/si-2-cap.txt
    # Second subprocess in the same pty: cursor should be past the
    # message, no new output expected.
    ppz read inbox --bare 2>/dev/null >> /tmp/si-2-cap.txt
    echo "end" >> /tmp/si-2-cap.txt
  ' </dev/null >/dev/null 2>&1

cat /tmp/si-2-cap.txt
rm -f /tmp/si-2-cap.txt
