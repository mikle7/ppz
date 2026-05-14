#!/usr/bin/env bash
# `/accounts/<id>` is now a router stub that 303-redirects to the Pipes
# tab. Each subsection (Pipes / Users / API keys) is its own URL,
# deep-linkable; visiting the bare org URL drops the user on the
# default tab. Pipes wins by virtue of being the operator's most
# common landing surface.
. /tests/lib/common.sh
auth_as_foo

org_id="$(cat /seed/org-alpha.txt)"

echo "--- bare org url returns a 303 redirect ---"
# --max-redirs 0 makes curl exit 47 when it sees a redirect; the
# `|| true` swallows it so the script's overall exit stays 0 (the
# harness has set -o pipefail).
{ curl_server "/accounts/$org_id" --max-redirs 0 -o /dev/null -w 'http=%{http_code}\n' || true; }

echo "--- redirect target is the pipes tab ---"
{ curl_server "/accounts/$org_id" --max-redirs 0 -s -D - -o /dev/null || true; } \
  | grep -i '^location:' \
  | sed -E 's/\r$//' \
  | head -1
