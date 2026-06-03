#!/usr/bin/env bash
# `subs read --raw` promises byte-faithful payloads with no separator. The
# per-target `=== <target> ===` banner must be suppressed under --raw (and
# --json), or the contract is broken.
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
export PPZ_SESSION=mysh
ppz_a source create room-a >/dev/null
ppz_a subs add room-a.inbox >/dev/null
ppz_a send room-a.inbox from-a >/dev/null
wait_for 20 "ppz_a ls | grep -q from-a" >/dev/null
n=$(ppz_a subs read --raw 2>/dev/null | grep -c '^=== ' || true)
echo "raw-separator-lines=$n"
