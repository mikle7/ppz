#!/usr/bin/env bash
. /tests/lib/common.sh
auth_as_foo
# Org alpha has 2 seeded api keys (alpha + alpha2). Each key prefix must
# appear on the org page exactly once.
org_id="$(cat /seed/org-alpha.txt)"
# Keys live on the API-keys tab now; the bare /accounts/<id> redirects
# to /accounts/<id>/pipes (the default landing).
curl_server "/accounts/$org_id/keys" \
  | grep -oE 'data-key-prefix="[^"]+"' \
  | wc -l \
  | tr -d ' '
