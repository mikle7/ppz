#!/usr/bin/env bash
# `ppz pipe destroy <name>` removes the pipe row + JetStream stream.
# After destroy, ls no longer shows the pipe.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create chat >/dev/null
ppz_a pipe create archive >/dev/null

echo "--- before destroy ---"
ppz_a ls | grep '^chat\.archive' | ls_normalize

ppz_a pipe destroy archive

echo "--- after destroy ---"
ppz_a ls | grep '^chat\.archive' || echo "no archive row"
