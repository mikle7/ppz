#!/usr/bin/env bash
# Phase 1.5: `ppz ls` includes uncollared (sourceless) pipes in its
# output. Pre-1.5 the listing was built by walking sources and
# joining their pipes; uncollared pipes have no source row to walk,
# so they were silently missing. Phase 1.5 must surface them.
#
# Setup: one uncollared pipe at root (`plaza`), one uncollared pipe
# at manifold `team-a` (`team-a.chat`), and one collared pipe
# (`alice.notes`). All three must appear in `ppz ls`.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1
ppz_a unset namespace >/dev/null 2>&1

# Uncollared at root.
ppz_a pipe create plaza >/dev/null

# Uncollared at manifold team-a.
ppz_a set namespace team-a >/dev/null
ppz_a pipe create chat >/dev/null
ppz_a unset namespace >/dev/null

# Collared on alice (the message-kind source auto-provisions alice.inbox
# too, but we only assert on the non-auto-provisioned alice.notes row
# below).
ppz_a source create alice >/dev/null
ppz_a pipe create alice.notes >/dev/null
ppz_a unset handle >/dev/null 2>&1

ppz_a ls | ls_normalize | grep -E '^(plaza|team-a\.chat|alice\.notes) ' | sort
