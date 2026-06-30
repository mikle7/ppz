#!/usr/bin/env bash
# The org page pipes table gains a NAMESPACE column header mirroring
# the `ppz ls` CLI layout. Asserting the FIRST `<th>` is
# `<th>namespace</th>` locks in column-1 position — the whole point of
# this feature is that NAMESPACE sits leftmost. Downstream tests cover
# the per-row cell contents.
. /tests/lib/common.sh
auth_as_foo

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null
ppz_a send chat.inbox "hello header" >/dev/null
wait_for 20 "ppz_a ls | grep -q 'hello header'" >/dev/null

PAGE="$(curl_server "/orgs/alpha")"

# Grab the first <th> inside the pipes table's thead — that's the
# column-1 header. Flattening newlines first guards against template
# whitespace shifts.
echo "$PAGE" \
  | tr -d '\n' \
  | grep -oE '<table id="pipes">[[:space:]]*<thead>[[:space:]]*<tr><th>[^<]+</th>' \
  | grep -oE '<th>[^<]+</th>' \
  | head -1
