#!/usr/bin/env bash
# Inside foo's own session (PPZ_SESSION=foo), foo.inbox is "self".
# `subs rm foo.inbox` is refused (non-zero exit) and the sub stays —
# an agent opting out of its own monitor is treated as misconfiguration.
# `--force` is the deliberate escape hatch.
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
PPZ_SESSION=desk ppz_a source create foo >/dev/null   # auto-subs foo.inbox under "foo"
export PPZ_SESSION=foo
ppz_a subs rm foo.inbox 2>/dev/null; rc=$?
[ "$rc" -ne 0 ] && echo "guard=refused" || echo "guard=ALLOWED-BUG"
echo "--- still subscribed ---"
ppz_a subs ls | ls_normalize | awk '{print $1}'
ppz_a subs rm --force foo.inbox; echo "force-rc=$?"
echo "--- after force ---"
ppz_a subs ls | ls_normalize | awk '{print $1}'
