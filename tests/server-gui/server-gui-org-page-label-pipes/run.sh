#!/usr/bin/env bash
# The org page table is one row per (source, pipe) pair — so the table
# is really listing pipes, not sources. The tab nav must mark "pipes"
# active, and the column that holds the source's handle must be
# labeled "source" (not "handle" or similar) so the row-shape reads
# as pipes-not-sources. Post-v0.34 the leftmost column is NAMESPACE
# (the pipe's manifold) and "source" is the second header — this
# test no longer pins which position "source" sits in, only that the
# labeled column exists.
. /tests/lib/common.sh
auth_as_foo

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create foo >/dev/null

# Bare /orgs/<id> redirects to the pipes tab — exactly where this
# table lives — so curl_server's -L follows and lands us here.
PAGE=$(curl_server "/orgs/alpha/pipes")

echo "--- tab nav marks pipes as active ---"
echo "$PAGE" | grep -oE 'data-active-tab="pipes"' | head -1

echo "--- source column header is present ---"
echo "$PAGE" | grep -oE '<th>source</th>' | head -1
