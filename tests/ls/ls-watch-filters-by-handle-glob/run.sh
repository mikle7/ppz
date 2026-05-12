#!/usr/bin/env bash
# `ppz ls --watch <pattern>...` filters which handles count as "matching"
# for both the immediate-unread check and the wakeup trigger. Patterns
# are filepath.Match-style globs against handle (not handle.pipe);
# multiple patterns OR-combine.
#
# This test verifies:
#   - non-matching sources don't appear in the snapshot
#   - non-matching traffic doesn't wake the watch
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create agent-one >/dev/null
ppz_a send agent-one.inbox "agent message" >/dev/null
ppz_a source create other >/dev/null
ppz_a send other.inbox "other message" >/dev/null
wait_for 20 "ppz_a ls 2>/dev/null | grep -q 'other message'" >/dev/null

# Both sources have unread. Watch with 'agent-*' should return immediately
# with ONLY agent-one's row — not 'other'.
echo "--- watch agent-* — only matching rows ---"
ppz_a ls --watch 'agent-*' | ls_normalize
