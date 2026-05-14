#!/usr/bin/env bash
# Each api-key row on /accounts/<id>/keys carries a `data-key-creator="<username>"`
# attribute so the dashboard can show who minted each key. Seeded keys:
# alpha-primary→foo, alpha-secondary→bar (per the seeder).
#
# We extract creator usernames in document order (alpha lists keys by
# created_at ASC, so alpha-primary precedes alpha-secondary).
. /tests/lib/common.sh
auth_as_foo
org_id="$(cat /seed/org-alpha.txt)"

curl_server "/accounts/$org_id/keys" \
  | grep -oE 'data-key-creator="[^"]+"' \
  | sed -E 's/data-key-creator="([^"]+)"/\1/'
