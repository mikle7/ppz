#!/usr/bin/env bash
# The org index page links to each org via its slug (the unique `name`
# column), not the UUID — slugs are user-friendly and stable across
# environments. The route at /accounts/{id} accepts both forms via resolveOrg,
# so behavior is identical; this test just locks the *href format* in.
. /tests/lib/common.sh
auth_as_bar

curl_server /dashboard \
  | grep -oE '<a href="/accounts/[^"]+"' \
  | sed -E 's/.*href="([^"]+)".*/\1/' \
  | sort
