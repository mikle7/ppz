#!/usr/bin/env bash
# A collared source created at manifold "xyz" must render its pipe
# rows with NAMESPACE="xyz". Mirrors the CLI's behaviour: NAMESPACE
# tracks the pipe's manifold, not the listing session's current
# namespace.
. /tests/lib/common.sh
auth_as_foo

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1
ppz_a set namespace xyz >/dev/null
ppz_a source create alice >/dev/null
ppz_a pipe create alice.notes >/dev/null
ppz_a unset namespace >/dev/null

PAGE="$(curl_server "/orgs/alpha")"

echo "$PAGE" \
  | tr '\n' ' ' \
  | sed -E 's/<tr /\n<tr /g' \
  | grep -E 'data-source-row="alice:notes:' \
  | grep -oE 'data-source-namespace="[^"]*"' \
  | sed -E 's/data-source-namespace="([^"]*)"/namespace=\1/'
