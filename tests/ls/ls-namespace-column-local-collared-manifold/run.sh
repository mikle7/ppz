#!/usr/bin/env bash
# `ppz ls` NAMESPACE column for a collared pipe created locally with
# session namespace = "xyz": NAMESPACE shows "xyz" (the manifold the
# pipe lives in), PIPE shows `<handle>.<pipe>` only — the manifold
# moves out of PIPE into its own column.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle    >/dev/null 2>&1
ppz_a set namespace xyz >/dev/null
ppz_a source create alice >/dev/null
ppz_a pipe create alice.notes >/dev/null
ppz_a unset namespace >/dev/null

ppz_a ls | awk '$2 == "alice.notes" {print "namespace=" $1 " pipe=" $2}'
