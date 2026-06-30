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
ppz_a source create chat >/dev/null
ppz_a send chat.inbox "this is a fairly long payload that would wrap onto two lines without the truncate rule applied" >/dev/null
wait_for 20 "ppz_a ls | grep -q 'fairly long payload'" >/dev/null

PAGE="$(curl_server "/orgs/alpha")"

# Project just the chat.inbox row (collapse the multi-line <tr> tag
# first), then assert the data-cell="payload" td carries the
# `cell-truncate` class. Boundary matcher (`\bcell-truncate\b`) so a
# future sibling class on the same cell doesn't trip a non-regression.
echo "$PAGE" \
  | tr '\n' ' ' \
  | sed -E 's/<tr /\n<tr /g' \
  | grep -E 'data-source-row="chat:inbox:' \
  | grep -oE '<td[^>]*data-cell="payload"[^>]*class="[^"]*\bcell-truncate\b[^"]*"' \
  | head -1 \
  | sed -E 's/.*/payload-has-cell-truncate=true/'
