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
# terminal create provisions inbox + stdin + stdout + stdctrl so we
# get a multi-pipe handle to exercise the recursive bit.
ppz_a terminal create cindy >/dev/null
# Plus a custom user-pipe so we cover the auto + user destroy path.
ppz_a pipe create cindy.archive >/dev/null

echo "--- before destroy ---"
ppz_a ls | ls_normalize | awk '$1 ~ /^cindy\./' | sort

ppz_a pipe destroy --recursive cindy

echo "--- after destroy ---"
ppz_a ls | ls_normalize | awk '$1 ~ /^cindy\./' | wc -l | tr -d ' '
