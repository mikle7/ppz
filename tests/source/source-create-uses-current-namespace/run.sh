#!/usr/bin/env bash
# Phase 1.5.1: `ppz source create X` creates the source at the
# session's current namespace. Pre-1.5.1 it always went to root.
# Sources at non-root manifold render with the manifold in front
# (e.g. `team-a.alice.inbox`).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a set namespace team-a >/dev/null
ppz_a source create alice >/dev/null
ppz_a ls | ls_normalize | grep -E '^(team-a\.)?alice\.' | cut -d' ' -f1 | sort
