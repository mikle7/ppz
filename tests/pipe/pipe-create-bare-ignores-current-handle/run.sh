#!/usr/bin/env bash
# Phase 1.5.1: bare `ppz pipe create LEAF` ALWAYS creates uncollared at
# current namespace. Current handle plays no role in destination routing
# for creates — that's a sender-identity concept used only by sends.
#
# Sets up the conflict: source `foo` exists (current_handle = foo as a
# side effect), then bare `ppz pipe create room` — must NOT auto-collar
# under foo. Must create uncollared `room` at root (no namespace set).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create foo >/dev/null
# foo is now current handle. No namespace set.
ppz_a pipe create room
