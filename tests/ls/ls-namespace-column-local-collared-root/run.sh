#!/usr/bin/env bash
# `ppz ls` carries a NAMESPACE column (leftmost). When a collared pipe
# is created locally with no session namespace set, the pipe lives at
# root → NAMESPACE renders as "-" (the table's missing-value glyph)
# and PIPE shows just `<handle>.<pipe>` (no manifold prefix).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset namespace >/dev/null 2>&1
ppz_a unset handle    >/dev/null 2>&1

ppz_a source create alice >/dev/null
ppz_a pipe create alice.notes >/dev/null

# Project NAMESPACE ($1) + PIPE ($2) for the alice.notes row. Raw output
# (no ls_normalize) so the NAMESPACE cell is visible.
ppz_a ls | awk '$2 == "alice.notes" {print "namespace=" $1 " pipe=" $2}'
