#!/usr/bin/env bash
# When foo mints a new api key via POST /accounts/<name>/keys, the row that
# results in /accounts/<id>/keys carries `data-key-creator="foo"`. Seeded
# alpha-primary already attributes to foo, so the post-mint listing
# carries foo's username for two of the three (alpha-primary + the
# new key); alpha-secondary stays bar.
. /tests/lib/common.sh
auth_as_foo
org_id="$(cat /seed/org-alpha.txt)"

# Mint a new key as foo.
curl_server "/accounts/$org_id/keys" -X POST --data-urlencode 'label=fresh-key' >/dev/null

# Count how many rows attribute to each user.
keys_html=$(curl_server "/accounts/$org_id/keys")
echo "creator=foo count=$(printf '%s' "$keys_html" | grep -oE 'data-key-creator="foo"' | wc -l | tr -d ' ')"
echo "creator=bar count=$(printf '%s' "$keys_html" | grep -oE 'data-key-creator="bar"' | wc -l | tr -d ' ')"
