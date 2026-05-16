#!/usr/bin/env bash
# `ppz await` with no positional args waits on the current handle's
# inbox. When a message has already been buffered, it returns
# immediately (level-triggered): banner to stderr, tabular drain to
# stdout, cursor advances.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a send chat.inbox "hello there" >/dev/null
wait_for 20 "ppz_a ls | grep -q 'hello there'" >/dev/null

# stderr (banner) discarded by the test harness automatically; we
# explicitly cat stdout for the drained body.
ppz_a await
