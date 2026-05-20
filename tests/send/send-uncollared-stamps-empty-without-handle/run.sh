#!/usr/bin/env bash
# Layer 2 (docs/specs/session-binding.md): uncollared sends with NO
# current handle set are REJECTED with E_NO_CURRENT_SOURCE.
#
# Before this spec, the daemon happily stamped envelope.sender="" and
# published anonymously, making it possible for sub-agents/sub-shells
# that lost PPZ_SESSION env to publish untraceable messages. The
# previous version of this fixture pinned that broken behavior; this
# version pins the fail-closed replacement.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a unset namespace >/dev/null
ppz_a pipe create room >/dev/null

err=$(mktemp)
ppz_a send room "anonymous shout" 2>"$err"
rc=$?

# Expect: failure with E_NO_CURRENT_SOURCE.
if grep -qE 'E_NO_CURRENT_SOURCE|no current source' "$err"; then
  echo "rejected: E_NO_CURRENT_SOURCE"
else
  echo "rejected: unexpected ($(head -1 "$err"))"
fi
echo "exit_code: $rc"

# Verify no message landed in room.
count=$(ppz_a reread room 2>/dev/null | grep -c 'anonymous shout' || true)
echo "room_count: ${count:-0}"

rm -f "$err"
