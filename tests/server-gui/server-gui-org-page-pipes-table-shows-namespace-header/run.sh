#!/usr/bin/env bash
# The org page pipes table gains a NAMESPACE column header mirroring
# the `ppz ls` CLI layout. Asserting the literal `<th>namespace</th>`
# locks in the new column's existence; downstream tests cover the
# per-row cell contents.
. /tests/lib/common.sh
auth_as_foo

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create chat >/dev/null
ppz_a send chat.inbox "hello header" >/dev/null
wait_for 20 "ppz_a ls | grep -q 'hello header'" >/dev/null

PAGE="$(curl_server "/orgs/alpha")"

# Project just the namespace th — exact-match keeps the assertion
# stable against unrelated header changes.
echo "$PAGE" \
  | grep -oE '<th>namespace</th>' \
  | head -1
