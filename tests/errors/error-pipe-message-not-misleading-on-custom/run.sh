#!/usr/bin/env bash
# RED for Track B (docs/AGENT_HARDENING.md): the E_INVALID_PIPE catalog-
# default message enumerates only {broadcast, inbox, stdin, stdout} as
# valid pipes — but custom pipes created via `ppz pipe create` are also
# valid. MoltHub's Charlie and Bob both hit this and walked away thinking
# custom pipes weren't supported.
#
# Trigger: send to a non-existent custom pipe on a real source. The
# daemon's stream-existence check (handlers.go around line 745) returns
# E_INVALID_PIPE with the catalog default message in this case.
#
# Assertions:
#   - The misleading enumeration "{broadcast, inbox, stdin, stdout}"
#     MUST NOT appear in the message.
#   - The actionable command "ppz pipe create" MUST appear.
#
# Both expressed as yes/no property checks so the implementer is free
# to pick any wording that satisfies both — the test doesn't pin the
# exact text.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create foo >/dev/null

# `nonexistent-pipe` is regex-valid (so we pass `natsubj.ValidatePipe`)
# but no `ppz pipe create foo.nonexistent-pipe` was run — the JetStream
# stream-existence check fails and the daemon returns E_INVALID_PIPE
# with the catalog default message.
err_msg=$(ppz_a send foo.nonexistent-pipe "test" 2>&1 | grep '^error:' | head -1)

# Property 1: the false enumerated set must NOT be in the message.
if echo "$err_msg" | grep -q "{broadcast, inbox, stdin, stdout}"; then
  echo "false-enum-present: yes"
else
  echo "false-enum-present: no"
fi

# Property 2: 'ppz pipe create' must be mentioned so users hitting
# this on a custom pipe see the actionable next step.
if echo "$err_msg" | grep -q "ppz pipe create"; then
  echo "mentions-pipe-create: yes"
else
  echo "mentions-pipe-create: no"
fi
