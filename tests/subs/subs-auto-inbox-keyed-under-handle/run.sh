#!/usr/bin/env bash
# `source create foo` auto-subscribes foo.inbox, keyed under the HANDLE
# ("foo"), not under the shell that ran the create. So:
#   - foo's own session (PPZ_SESSION=foo) sees foo.inbox in subs ls
#   - the creating shell (PPZ_SESSION=desk) does NOT — the sub belongs
#     to the agent, and must not leak into the operator's personal list.
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
PPZ_SESSION=desk ppz_a source create foo >/dev/null
echo "--- foo session ---"
PPZ_SESSION=foo ppz_a subs ls | ls_normalize | awk '{print $1}'
echo "--- creating shell session ---"
PPZ_SESSION=desk ppz_a subs ls | ls_normalize | awk '{print $1}'
