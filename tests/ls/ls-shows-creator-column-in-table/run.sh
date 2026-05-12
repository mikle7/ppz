#!/usr/bin/env bash
# `ppz ls` table layout: PIPE UNREAD BUFFERED LAST PAYLOAD CREATOR.
# CREATOR is rightmost — the username that owns the (source, pipe).
# alpha-primary→foo creates `chat`; the table includes a CREATOR column
# header and every row trails with `foo`.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create chat >/dev/null
ppz_a send chat.inbox "hello world" >/dev/null
wait_for 20 "ppz_a ls | grep -q hello" >/dev/null

# Normalise variable-width whitespace + relative time. Header row stays
# in (we want to see CREATOR as the last header), so use sed rather than
# the existing ls_normalize helper which strips it.
ppz_a ls \
  | sed -E 's/[[:space:]]+/ /g' \
  | sed -E 's/(just now|[0-9]+ (seconds?|minutes?|hours?|days?) ago)/RELATIVE/'
