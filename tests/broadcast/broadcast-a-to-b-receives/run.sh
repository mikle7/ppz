#!/usr/bin/env bash
. /tests/lib/common.sh
# daemon-a and daemon-b both belong to org alpha (different api keys).
# 'a' creates the pipe and broadcasts; 'b' must see the latest broadcast via
# its own ls (which queries the server, which mirrors broadcasts into the
# pipes table).
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)"  >/dev/null
ppz_b daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha2)" >/dev/null
ppz_a source create remote >/dev/null
ppz_a broadcast -m "hi from a" >/dev/null
wait_for 20 "ppz_b ls | grep -q 'hi from a'" >/dev/null
ppz_b ls | ls_normalize
