#!/usr/bin/env bash
# `ppz pipe destroy *` (bare-star glob) destroys every user-created
# pipe — uncollared pipes AND user-created collared pipes — but
# leaves auto-pipes (inbox/stdin/stdout/stdctrl) and the sources
# themselves intact. Pattern parity with `ppz source destroy *`
# without the source-destroying semantics.
#
# Pre-fix: bare "*" hits ValidatePipe and returns E_INVALID_PIPE so
# nothing happens.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a unset namespace >/dev/null

# Set up: one source with an auto-pipe inbox, plus a user-created
# pipe on that source, plus a standalone uncollared pipe.
ppz_a source create chat >/dev/null
ppz_a unset handle >/dev/null
ppz_a pipe create chat.archive >/dev/null
ppz_a pipe create room >/dev/null

echo "--- before destroy ---"
ppz_a ls | awk '{print $1}' | grep -v '^PIPE$' | sort

# Glob destroy. Quoted so the host shell doesn't expand against cwd.
ppz_a pipe destroy '*' 2>&1 | sort

echo "--- after destroy ---"
ppz_a ls | awk '{print $1}' | grep -v '^PIPE$' | sort
