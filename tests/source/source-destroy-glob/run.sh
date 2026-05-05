#!/usr/bin/env bash
# `ppz source destroy 'agent-*'` destroys all sources matching the glob,
# leaving non-matching sources intact.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create agent-one >/dev/null
ppz_a source create agent-two >/dev/null
ppz_a source create other >/dev/null

echo "--- before destroy ---"
ppz_a ls | ls_normalize | grep -E '^(agent-one|agent-two|other)\.'

ppz_a source destroy 'agent-*' | sort

echo "--- after destroy ---"
ppz_a ls | ls_normalize | grep -E '^(agent-one|agent-two|other)\.' || echo "no agent rows"
