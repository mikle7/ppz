#!/usr/bin/env bash
# `ppz pipe destroy <name>` removes the pipe row + JetStream stream.
# After destroy, ls no longer shows the pipe.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a pipe create chat.archive >/dev/null

echo "--- before destroy ---"
ppz_a ls | ls_normalize | grep '^chat\.archive'

ppz_a pipe destroy chat.archive

echo "--- after destroy ---"
ppz_a ls | ls_normalize | grep '^chat\.archive' || echo "no archive row"
