#!/usr/bin/env bash
# AS-1 (after-fix): the prod bug repro. Cindy's pty spawns a sub-shell
# with env-effectively-stripped that sends to david. Before this spec,
# the daemon stamps empty sender → david sees an untraceable message.
# After Layer 1+2:
#   - Ancestor walk resolves the subprocess's session to cindy.
#   - Auto-write populates current["agent:cindy"] = "cindy".
#   - Send handler resolves sender=cindy, publishes with that sender.
#   - David receives the message with `sender=cindy`.
#
# The "before" half of this fixture is the existing
# `tests/send/send-uncollared-stamps-empty-without-handle/` — that
# fixture encodes today's broken behavior and must be updated to
# expect E_NO_CURRENT_SOURCE (or to expect cindy-as-sender) when this
# spec lands.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a unset namespace >/dev/null

# david is the recipient; created on daemon A (single-daemon org).
ppz_a source create david >/dev/null
ppz_a unset handle >/dev/null

# The smoking gun: env-effectively-stripped subprocess inside cindy's
# pty sends to david. We don't strip PPZ_IPC_SOCKET (otherwise ppz
# couldn't find the daemon at all); we strip PPZ_SESSION, which is
# the variable the bug is about. PATH and PPZ_IPC_SOCKET are
# inherited normally.
PPZ_IPC_SOCKET="$PPZ_DAEMON_A_SOCK" \
  ppz terminal share cindy -- sh -c '
    env -u PPZ_SESSION ppz send david "hello-from-cindys-sub-shell" 2>&1 > /tmp/as-1-send.txt
  ' </dev/null >/dev/null 2>&1

# Show send outcome (success line OR error).
grep -E "^(sent|ppz)" /tmp/as-1-send.txt | head -1 \
  | sed -E 's/id=[a-f0-9]{8}/id=ID8/; s/bytes=[0-9]+/bytes=N/'

# Show what david's inbox received — specifically the sender field.
ppz_a reread david.inbox --json -l 1 2>/dev/null | head -1 \
  | sed -E 's/.*"sender":"([^"]*)".*/sender=\1/'

rm -f /tmp/as-1-send.txt
