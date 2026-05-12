#!/usr/bin/env bash
# `ppz reread` is the forensic / replay verb: it ignores the session
# cursor entirely and surfaces all retained messages, with optional
# filters layered on top.
#
# -l N returns the N MOST RECENT messages (tail-N semantics, like
# `tail -n N`), still printed oldest-first. Filter order is:
# --since → --skip → -l, so -l is applied last.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create chat >/dev/null
for i in 1 2 3 4 5; do
  ppz_a send chat.inbox "msg-$i" >/dev/null
done
wait_for 20 "ppz_a ls | grep -q msg-5" >/dev/null

ppz_a reread --bare chat.inbox -l 2
