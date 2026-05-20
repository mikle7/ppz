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

# Create cindy as message-kind and pre-send to her inbox. Use bare
# `terminal share --` afterwards so we don't try to re-create cindy
# as a fresh PTY source (which would conflict).
ppz_a source create cindy >/dev/null
ppz_a send --from pubsub cindy "msg-for-cursor-test" >/dev/null

PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" \
  ppz terminal share -- sh -c '
    # First subprocess: drain new messages. env -u strips the env
    # pins so resolution depends on daemon-side binding.
    echo "sub1:" > /tmp/si-2-cap.txt
    env -u PPZ_CURRENT_HANDLE -u PPZ_SESSION ppz read inbox --bare 2>/dev/null >> /tmp/si-2-cap.txt
    echo "sub2:" >> /tmp/si-2-cap.txt
    # Second subprocess: cursor should be past the message, no output.
    env -u PPZ_CURRENT_HANDLE -u PPZ_SESSION ppz read inbox --bare 2>/dev/null >> /tmp/si-2-cap.txt
    echo "end" >> /tmp/si-2-cap.txt
  ' </dev/null >/dev/null 2>&1

cat /tmp/si-2-cap.txt
rm -f /tmp/si-2-cap.txt
