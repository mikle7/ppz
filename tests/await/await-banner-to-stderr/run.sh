#!/usr/bin/env bash
# Arrival banner goes to stderr, not stdout, so `ppz await … | <consumer>`
# pipelines see only message bodies. We merge stderr into a captured file
# and assert it contains the banner; stdout (the harness's diff target)
# carries only the drained body.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a send chat.inbox "body line" >/dev/null
wait_for 20 "ppz_a ls | grep -q 'body line'" >/dev/null

ERR=/tmp/await-banner.err
ppz_a await chat.inbox 2>"$ERR"

# Verify the banner landed on stderr.
if grep -qE 'messages arrived on chat\.inbox' "$ERR"; then
  echo "BANNER_ON_STDERR=yes"
else
  echo "BANNER_ON_STDERR=no"
fi
