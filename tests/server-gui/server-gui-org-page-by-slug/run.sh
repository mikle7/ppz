#!/usr/bin/env bash
# /orgs/{id} must accept the org name as an alias for its UUID.
# Asserts the /orgs/alpha page renders the same view as /orgs/<alpha-uuid>
# by counting the seeded api-key rows (alpha has two: alpha-primary +
# alpha-secondary).
. /tests/lib/common.sh
auth_as_foo

# Keys live on the API-keys tab; bare /orgs/alpha redirects to the
# pipes tab now, so go straight to /orgs/alpha/keys for the count.
curl_server "/orgs/alpha/keys" \
  | grep -oE 'data-key-prefix="[^"]+"' \
  | wc -l \
  | tr -d ' '
