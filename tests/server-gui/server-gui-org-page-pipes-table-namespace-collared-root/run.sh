#!/usr/bin/env bash
# A collared source at root namespace must render its pipe rows with
# NAMESPACE="-" (the convention the CLI's ppz ls NAMESPACE column
# uses). Asserted via the stable data-source-namespace attribute on
# each row so the test isn't sensitive to visible-cell wrapping.
. /tests/lib/common.sh
auth_as_foo

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset namespace >/dev/null 2>&1
ppz_a unset handle    >/dev/null 2>&1
ppz_a source create alice >/dev/null
ppz_a pipe create alice.notes >/dev/null

PAGE="$(curl_server "/orgs/alpha")"

# Project the namespace attribute on the alice.notes row only. The
# <tr> opening tag wraps over multiple physical lines in the
# rendered HTML (Go template preserves source whitespace), so we
# flatten the page to one line and split per row before grepping
# for the row marker.
echo "$PAGE" \
  | tr '\n' ' ' \
  | sed -E 's/<tr /\n<tr /g' \
  | grep -E 'data-source-row="alice:notes:' \
  | grep -oE 'data-source-namespace="[^"]*"' \
  | sed -E 's/data-source-namespace="([^"]*)"/namespace=\1/'
