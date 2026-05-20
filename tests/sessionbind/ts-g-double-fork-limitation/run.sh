#!/usr/bin/env bash
# TS-G: documented limitation — a double-fork-detached subprocess
# (reparented to init, no ppid trail back to the share) does NOT
# resolve via the binding. The acceptance behavior is "fails CLEANLY
# with E_NO_CURRENT_SOURCE", not a confusing wrong-handle stamp.
#
# Workaround for users who genuinely need this: set PPZ_SESSION
# inline.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a unset namespace >/dev/null

# setsid -f double-detaches: forks, makes the child a session leader,
# and exits the parent — child's ppid becomes 1 (init). The original
# share pid is no longer in the child's ancestor chain.
#
# `ppz status` from inside that orphaned context shouldn't see cindy.
# It should report empty current (or a sid-N fallback) — NOT cindy.
PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" \
  ppz terminal share cindy -- sh -c '
    setsid -f sh -c "ppz status 2>&1 > /tmp/ts-g-cap.txt" 2>/dev/null
    # Give the detached child a moment to write.
    sleep 1
  ' </dev/null >/dev/null 2>&1

# Acceptance: detached child does NOT report cindy as current.
if grep -qE "^(current source|namespace): cindy" /tmp/ts-g-cap.txt 2>/dev/null; then
  echo "result: incorrectly resolved to cindy (limitation should hold)"
else
  echo "result: clean fallback (not cindy)"
fi
rm -f /tmp/ts-g-cap.txt
