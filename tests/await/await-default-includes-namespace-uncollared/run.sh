#!/usr/bin/env bash
# `ppz await` with no args watches the current handle's inbox AND every
# uncollared pipe at the current namespace. This is the user-reported
# scenario: a room-style chat lives as an uncollared pipe; an agent in
# a session with `current=foo` should pick up traffic on both
# `foo.inbox` and `room` without naming them.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create foo >/dev/null
ppz_a set handle foo >/dev/null
ppz_a pipe create room >/dev/null
# Hit only the uncollared pipe to confirm the default expansion
# includes it (the inbox path is already covered by
# await-default-is-current-inbox).
ppz_a send room "namespace hello" >/dev/null
wait_for 20 "ppz_a ls | grep -q 'namespace hello'" >/dev/null

ppz_a await
