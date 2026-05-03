#!/usr/bin/env bash
# The org page table is one row per (source, pipe) pair — so the table is
# really listing pipes, not sources. The title must say "Pipes" and the
# first column header (which holds the source's handle) must say "source".
# Otherwise the page reads as "sources with a handle column" which is
# confusing — the row count != source count.
. /tests/lib/common.sh
auth_as_foo

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create foo >/dev/null

# Bare /orgs/<id> redirects to the pipes tab — exactly where this
# table lives — so curl_server's -L follows and lands us here.
PAGE=$(curl_server "/orgs/alpha/pipes")

echo "--- tab nav marks pipes as active ---"
echo "$PAGE" | grep -oE 'data-active-tab="pipes"' | head -1

echo "--- column header (first th) ---"
# Grab the first <th> inside the table — that's the column-1 header.
echo "$PAGE" | tr -d '\n' | grep -oE '<table id="(sources|pipes)">[[:space:]]*<thead>[[:space:]]*<tr><th>[^<]+</th>' \
  | grep -oE '<th>[^<]+</th>' \
  | head -1
