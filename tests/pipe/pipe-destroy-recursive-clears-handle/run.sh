#!/usr/bin/env bash
# `ppz pipe destroy --recursive HANDLE` destroys every pipe under
# HANDLE (auto-pipes + custom pipes) and removes the source row.
# Replacement for `ppz source destroy HANDLE` from the user's
# point of view — the underlying IPC verb is IPCSourceDestroy
# (which has its own coverage via tests/source/source-destroy-*),
# but this fixture pins the new --recursive flag-parsing path
# in internal/cli/pipe.go specifically.
#
# Locked decision #21.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
# source create provisions the inbox auto-pipe; the custom pipe below
# makes this a multi-pipe handle (auto + user) so the recursive
# destroy is exercised across both pipe origins.
ppz_a source create cindy >/dev/null
# Plus a custom user-pipe so we cover the auto + user destroy path.
ppz_a pipe create cindy.archive >/dev/null

echo "--- before destroy ---"
ppz_a ls | ls_normalize | awk '$1 ~ /^cindy\./' | sort

ppz_a pipe destroy --recursive cindy

echo "--- after destroy ---"
# Disable pipefail inside the subshell so grep -v in ls_normalize
# returning 1 (empty input after destroy → no "non-matching" lines)
# doesn't poison the pipeline exit code.
(set +o pipefail; ppz_a ls | ls_normalize | awk '$1 ~ /^cindy\./' | wc -l | tr -d ' ')
