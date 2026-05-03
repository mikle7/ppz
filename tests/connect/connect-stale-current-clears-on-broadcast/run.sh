#!/usr/bin/env bash
# Repro for the "fresh window inherits stale current" footgun:
#
# 1. Session A connects to source 'foo' (sets current[A]=foo).
# 2. The source is deleted out from under us (could happen via another
#    org member, GUI, db cleanup, etc. — easiest in a test is direct DB).
# 3. Session A runs `ppz broadcast` with no explicit handle.
#
# Without auto-clear, broadcast resolves current=foo, the server says
# "doesn't exist", and the user sees a confusing E_SOURCE_NOT_FOUND
# pointing at a handle they don't remember setting.
#
# After fix: detect the stale entry, clear it, return E_NO_CURRENT_SOURCE
# (the actionable error: "set one with `ppz connect`"). Subsequent calls
# in this session see no current — clean state.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
PPZ_SESSION=tab-a ppz_a source create foo >/dev/null

# Yank the source out from under the daemon. TRUNCATE CASCADE drops the
# pipes-table FK chain too. After this, the server doesn't know about
# 'foo' even though the daemon still has it cached as tab-a's current.
PGPASSWORD=ppz psql -h postgres -U postgres -d ppz \
  -c "TRUNCATE TABLE sources CASCADE" >/dev/null

echo "--- broadcast with stale current — should be E_NO_CURRENT_SOURCE, not E_SOURCE_NOT_FOUND ---"
broadcast_out=$(PPZ_SESSION=tab-a ppz_a broadcast -m "hi" 2>&1)
broadcast_rc=$?
echo "$broadcast_out" | grep '^error:' || true
echo "broadcast_exit=$broadcast_rc"

echo "--- status after broadcast — current should be cleared ---"
PPZ_SESSION=tab-a ppz_a status | grep '^current source:'
