#!/usr/bin/env bash
# An uncollared pipe at manifold "xyz" must render NAMESPACE="xyz"
# AND drop the manifold prefix from the PIPE cell (the leaf alone).
# Mirrors the CLI: once NAMESPACE owns the manifold fact, PIPE shows
# just the pipe name. The data-source-row marker's pipe slot
# therefore changes from the pre-feature combined `xyz.testroom` to
# the leaf-only `testroom`.
. /tests/lib/common.sh
auth_as_foo

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle    >/dev/null 2>&1
ppz_a set namespace xyz >/dev/null
ppz_a pipe create testroom >/dev/null
ppz_a unset namespace >/dev/null
wait_for 20 "ppz_a ls | grep -q testroom" >/dev/null

PAGE="$(curl_server "/orgs/alpha")"

# 1) Namespace attribute on the testroom row.
echo "$PAGE" \
  | tr '>' '\n' \
  | grep -E 'data-source-row=":testroom:' \
  | grep -oE 'data-source-namespace="[^"]*"' \
  | sed -E 's/data-source-namespace="([^"]*)"/namespace=\1/'

# 2) Row marker carries the bare leaf in the pipe slot, not the
#    pre-feature `xyz.testroom` combined form.
echo "$PAGE" \
  | grep -oE 'data-source-row=":testroom::"' \
  | head -1
