#!/usr/bin/env bash
# `/dashboard` is the new home for the operator org-index — what
# used to live at `/`. The org-list contract (one `data-org="<name>"`
# per row) has to keep working from the new URL so existing UI flows
# (header brand link, post-create-org redirect) land somewhere
# useful.
. /tests/lib/common.sh
auth_as_bar

echo "--- /dashboard lists seeded orgs ---"
curl_server "/dashboard" \
  | grep -oE 'data-org="[^"]+"' \
  | sed -E 's/data-org="([^"]+)"/\1/' \
  | sort
