#!/usr/bin/env bash
# `connect`/`switch`/`create` set the daemon's "current source" — but only
# for the calling session (tty / $PPZ_SESSION). A second terminal window
# (different session) MUST NOT inherit the first session's current.
#
# Without this, opening a new terminal silently inherits a current that
# was set hours ago somewhere else — surprising and often stale, and
# `ppz broadcast` ends up targeting the wrong handle.
#
# Mirrors the per-session cursor model already in place at
# internal/daemon/cursors.go.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
PPZ_SESSION=tab-a ppz_a source create foo >/dev/null

echo "--- tab-a status (should show foo) ---"
PPZ_SESSION=tab-a ppz_a status | grep '^current source:'

echo "--- tab-b status (separate session — must NOT inherit) ---"
PPZ_SESSION=tab-b ppz_a status | grep '^current source:'

echo "--- tab-a still shows foo (sanity: A's current didn't get clobbered) ---"
PPZ_SESSION=tab-a ppz_a status | grep '^current source:'
