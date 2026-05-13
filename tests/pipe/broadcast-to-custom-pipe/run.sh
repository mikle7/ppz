#!/usr/bin/env bash
# A custom pipe round-trips messages: send to <h>.<custom-pipe>, read
# them back. Same wire as inbox/stdout — the only difference is the
# pipe name carved out by the user.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a pipe create archive >/dev/null
ppz_a send chat.archive "long-term-1" >/dev/null
ppz_a send chat.archive "long-term-2" >/dev/null
wait_for 20 "ppz_a reread chat.archive --json | jq -r '.payload' | grep -q long-term-2" >/dev/null

ppz_a reread chat.archive
