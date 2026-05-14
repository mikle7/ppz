#!/usr/bin/env bash
# Phase 1.5.1: the collision rule is scoped to a manifold. Source `foo`
# at root + uncollared pipe `foo` at manifold `team-a` are different
# paths (`<acct>.foo.inbox` vs `<acct>.team-a.foo`) — no collision.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a source create foo >/dev/null
ppz_a unset handle >/dev/null
ppz_a set namespace team-a >/dev/null
ppz_a pipe create foo
ppz_a ls | grep -E '^(foo\.inbox|team-a\.foo) ' | sed -E 's/[[:space:]]+/ /g' | cut -d' ' -f1 | sort
