#!/usr/bin/env bash
# Bare `inbox` resolves to the current source's inbox pipe.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a send chat.inbox "inbox hello" >/dev/null

wait_for 20 "ppz_a ls | grep -q 'inbox hello'" >/dev/null
ppz_a read inbox
