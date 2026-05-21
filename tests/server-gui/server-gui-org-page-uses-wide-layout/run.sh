#!/usr/bin/env bash
# Fix: the org page's pipes table is constrained by the default
# `main { max-width: 960px }` rule, leaving large blank gutters on
# desktops and forcing PIPE/PAYLOAD cells to wrap onto multiple
# lines. The org page opts into a wide-layout container by adding
# `class="wide"` to its <main> element; matching CSS bumps the
# max-width so the table can use the available horizontal space.
. /tests/lib/common.sh
auth_as_foo

PAGE="$(curl_server "/orgs/alpha")"

# The opening <main> tag must carry the "wide" layout marker.
# Tolerate other classes around it (e.g. `class="page wide"`).
echo "$PAGE" \
  | grep -oE '<main[^>]*class="[^"]*\bwide\b[^"]*"' \
  | head -1 \
  | sed -E 's/.*class="([^"]*)".*/main-class=\1/'
