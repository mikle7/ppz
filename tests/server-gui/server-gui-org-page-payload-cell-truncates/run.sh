#!/usr/bin/env bash
# Fix: long payloads in the org pipes table currently wrap onto
# multiple lines (see the heartbeat JSON preview and the stdout
# terminal escape preview in the live screenshot). Wrapping makes
# rows visually noisy and unaligned. Truncate with an ellipsis
# instead — full payload remains available via the pipe detail page.
#
# The payload <td> must carry a stable `cell-truncate` class so the
# matching CSS rule (white-space: nowrap; overflow: hidden;
# text-overflow: ellipsis) can target it. Asserting the class is
# present is the structural part of the contract; visual truncation
# is verified manually against the rendered page.
. /tests/lib/common.sh
auth_as_foo

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create chat >/dev/null
ppz_a send chat.inbox "this is a fairly long payload that would wrap onto two lines without the truncate rule applied" >/dev/null
wait_for 20 "ppz_a ls | grep -q 'fairly long payload'" >/dev/null

PAGE="$(curl_server "/orgs/alpha")"

# Project just the chat.inbox row (collapse the multi-line <tr> tag
# first), then extract the data-cell="payload" td's class list.
echo "$PAGE" \
  | tr '\n' ' ' \
  | sed -E 's/<tr /\n<tr /g' \
  | grep -E 'data-source-row="chat:inbox:' \
  | grep -oE '<td[^>]*data-cell="payload"[^>]*class="[^"]*"' \
  | grep -oE 'class="[^"]*"' \
  | sed -E 's/class="([^"]*)"/payload-class=\1/' \
  | head -1
