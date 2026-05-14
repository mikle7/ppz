#!/usr/bin/env bash
# /accounts/{id} must accept the org name as an alias for its UUID.
# Asserts the /accounts/alpha page renders the same view as /accounts/<alpha-uuid>
# by counting the seeded api-key rows (alpha has two: alpha-primary +
# alpha-secondary).
. /tests/lib/common.sh
auth_as_foo

# Keys live on the API-keys tab; bare /accounts/alpha redirects to the
# pipes tab now, so go straight to /accounts/alpha/keys for the count.
curl_server "/accounts/alpha/keys" \
  | grep -oE 'data-key-prefix="[^"]+"' \
  | wc -l \
  | tr -d ' '
