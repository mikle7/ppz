#!/usr/bin/env bash
# Fix: the LAST MESSAGE header and cell were wrapping onto two lines
# at the default 960px container (see report screenshot). With the
# wider layout, the cell text "5 minutes ago" still risks wrapping
# if the column is narrow because "5" + "minutes" + "ago" are three
# words. Apply `cell-nowrap` to the last-message <td> so it stays
# on one line; matching CSS rule has `white-space: nowrap`.
#
# Test asserts the class marker is on the last-message cell — visual
# nowrap behaviour is provided by the CSS rule.
. /tests/lib/common.sh
auth_as_foo

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create chat >/dev/null
ppz_a send chat.inbox "row with last-message" >/dev/null
wait_for 20 "ppz_a ls | grep -q 'row with last-message'" >/dev/null

PAGE="$(curl_server "/orgs/alpha")"

# Project the chat.inbox row's last-message cell. Boundary matcher
# on `\bcell-nowrap\b` so a future sibling class doesn't trip a
# non-regression.
echo "$PAGE" \
  | tr '\n' ' ' \
  | sed -E 's/<tr /\n<tr /g' \
  | grep -E 'data-source-row="chat:inbox:' \
  | grep -oE '<td[^>]*data-cell="last-message"[^>]*class="[^"]*\bcell-nowrap\b[^"]*"' \
  | head -1 \
  | sed -E 's/.*/last-message-has-cell-nowrap=true/'
