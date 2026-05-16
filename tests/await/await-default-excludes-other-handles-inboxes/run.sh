#!/usr/bin/env bash
# `ppz await` (default) must NOT include other handles' collared
# pipes. Setup: handles `foo` (current) and `bar` both have inboxes;
# uncollared `room` exists at root. Publish to `bar.inbox` and `room`.
# Default await should drain `room` but leave `bar.inbox` unread.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create foo >/dev/null
ppz_a source create bar >/dev/null
ppz_a set handle foo >/dev/null
ppz_a pipe create room >/dev/null

ppz_a send bar.inbox "noise from another inbox" >/dev/null
ppz_a send room "drain me" >/dev/null
wait_for 20 "ppz_a ls | grep -q 'drain me'" >/dev/null

ppz_a await

echo "--- ls after await ---"
ppz_a ls | ls_normalize | grep -E '^(bar\.inbox|foo\.inbox|room) ' | sort
