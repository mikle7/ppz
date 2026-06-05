#!/usr/bin/env bash
# `subs ls --json` stays FLAT — one object per matched pipe, same base
# shape as `ls --watch --json` — but each row gains `matched_by`: the
# subscription(s) that surfaced it. The human tree is presentation only;
# JSON consumers get attribution as a field, never as nesting.
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1
ppz_a pipe create test-1 >/dev/null
ppz_a pipe create test-2 >/dev/null
export PPZ_SESSION=mysh
ppz_a subs add 'test-%' >/dev/null
echo "--- subs ls --json: flat rows carry matched_by ---"
ppz_a subs ls --json | jq -c '{pipe, matched_by}' | sort
