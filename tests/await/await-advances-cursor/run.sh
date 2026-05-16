#!/usr/bin/env bash
# After `ppz await` drains a pipe, the calling session's cursor must
# advance — same as `ppz read`. Verify by checking unread = 0 in `ppz ls`
# after the await.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1
ppz_a pipe create signal >/dev/null
ppz_a send signal "consumed" >/dev/null
wait_for 20 "ppz_a ls | grep -q consumed" >/dev/null

ppz_a await signal >/dev/null 2>&1

echo "--- ls after await ---"
ppz_a ls | ls_normalize | grep -E '^signal '
