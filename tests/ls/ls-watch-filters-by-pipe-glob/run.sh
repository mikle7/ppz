#!/usr/bin/env bash
# `ppz ls --watch <pattern>` should match patterns against the FULL
# `<handle>.<pipe>` target, not only against the handle. Today the
# matcher is handle-only, so a sensible-looking invocation like
# `ppz ls --watch '*.stdout'` matches no handles, returns no immediate
# unread, and blocks indefinitely — even when there's plainly unread
# on a stdout pipe.
#
# Repro: two pty sources with stdout activity, watch `*.stdout`. We
# expect both `.stdout` rows back; inbox / stdin / stdctrl rows
# from the same handles should be filtered out.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal share apple  -- printf "stdout-from-apple"  >/dev/null
ppz_a terminal share banana -- printf "stdout-from-banana" >/dev/null
# Wait for both stdout pipes to land in ls (BUFFERED > 0 means the
# pty bytes have been published).
wait_for 20 "ppz_a ls | awk '\$1 == \"banana.stdout\" && \$3+0 > 0'" >/dev/null
wait_for 20 "ppz_a ls | awk '\$1 == \"apple.stdout\"  && \$3+0 > 0'" >/dev/null

echo "--- watch '*.stdout' (quoted glob) — returns immediately, only stdout rows ---"
ppz_a ls --watch '*.stdout' | ls_normalize | sort

echo "--- watch '%stdout' (sql alias, unquoted in zsh) — same result ---"
ppz_a ls --watch '%stdout' | ls_normalize | sort
