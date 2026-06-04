#!/usr/bin/env bash
# `source create H` makes the creating session BECOME H (sets its current
# handle) and auto-subscribes H.inbox. The inbox is therefore visible:
#   - in the creating session itself — it's now operating as H, so a plain
#     `ppz subs ls/wait` from the shell that ran `source create` just works
#     (this is the spec's "agent's own inbox auto-subscribed at create time")
#   - and under H's own session key (PPZ_SESSION=H), for subprocesses.
# (The no-leak case — operator NOT seeing an agent's inbox — applies to the
# pty paths `terminal share` / `agent create`; see subs-auto-inbox-on-
# terminal-share.)
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
PPZ_SESSION=desk ppz_a source create foo >/dev/null
echo "--- creating session (desk → became foo) ---"
PPZ_SESSION=desk ppz_a subs ls | ls_normalize | awk '{print $1}'
echo "--- handle session (foo) ---"
PPZ_SESSION=foo ppz_a subs ls | ls_normalize | awk '{print $1}'
