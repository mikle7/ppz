#!/usr/bin/env bash
# `ppz await` (default) only watches uncollared pipes at the CURRENT
# namespace. Set namespace `team-a`, create `team-a.chat` and a root
# `lobby`. Publish to both. Default await should drain `team-a.chat`
# and leave `lobby` unread.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create foo >/dev/null
ppz_a set handle foo >/dev/null

# Root-namespace uncollared.
ppz_a unset namespace >/dev/null 2>&1
ppz_a pipe create lobby >/dev/null
ppz_a send lobby "root noise" >/dev/null

# Team-a namespace uncollared.
ppz_a set namespace team-a >/dev/null
ppz_a pipe create chat >/dev/null
ppz_a send chat "team chat" >/dev/null

wait_for 20 "ppz_a ls | grep -q 'team chat'" >/dev/null

# Current namespace = team-a; await should drain team-a.chat only.
ppz_a await

echo "--- ls after await ---"
ppz_a unset namespace >/dev/null 2>&1
ppz_a ls | ls_normalize | grep -E '^(lobby|team-a\.chat) ' | sort
