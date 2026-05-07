#!/usr/bin/env bash
# `--eof` opts out of the lines-by-default streaming mode: read the
# entire stdin into one buffer and publish as a single message.
# Preserves the "send my multi-line block as one logical message"
# case (e.g. an agent's prepared response).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create foo >/dev/null

printf 'first\nsecond\nthird\n' | ppz_a broadcast --eof >/dev/null
wait_for 20 "ppz_a ls | ls_normalize | grep -q '^foo.broadcast 1 1'" >/dev/null

echo "--- ls reports 1 buffered message (atomic) ---"
ppz_a ls | ls_normalize | grep '^foo.broadcast'

echo "--- reread emits a single multi-line payload, newlines preserved ---"
ppz_a reread --bare foo.broadcast
