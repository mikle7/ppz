#!/usr/bin/env bash
# THE FLAW (reported 2026-06): a glob sub like `test-%` is INVISIBLE in
# `subs ls` — only its read-time expansion (test-1, test-2, ...) shows,
# indistinguishable from literal subs, with no way to tell which pattern
# surfaced them. `subs rm test-1` then silently no-ops (test-1 isn't a
# stored subject; `test-%` is) and re-expands on the next ls.
#
# NEW: render a pattern as a PARENT row with its matched pipes as indented
# CHILD rows. Restores the spec invariant "every subscribed subject appears
# as a row" and makes attribution visible — you can see test-1..3 belong to
# the `test-%` subscription.
#
# Sources created with no handle so the matched pipes are uncollared (flat),
# matching the reported example. The subs live in session 'mysh' which has
# no auto-subscribed inbox, so the pattern block is the whole output.
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1
ppz_a pipe create test-1 >/dev/null
ppz_a pipe create test-2 >/dev/null
ppz_a pipe create test-3 >/dev/null
export PPZ_SESSION=mysh
ppz_a subs add 'test-%' >/dev/null
echo "--- subs ls: pattern parent + matched children ---"
ppz_a subs ls | ls_normalize
