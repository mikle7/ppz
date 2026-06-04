#!/usr/bin/env bash
# Removing a LITERAL sub that is ALSO matched by a still-present pattern
# (review #2). The literal really is removed, but the pipe re-expands under
# the surviving pattern — so the feedback must say so, rather than a bare
# `removed:` that falsely implies the pipe is gone. That bare message would
# re-create the exact "I removed the row I can see and it came back"
# confusion this feature sets out to kill.
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1
ppz_a pipe create test-1 >/dev/null
export PPZ_SESSION=mysh
ppz_a subs add 'test-%' >/dev/null
ppz_a subs add test-1 >/dev/null

echo "--- rm the literal while pattern still covers it ---"
ppz_a subs rm test-1; echo "rc=$?"
echo "--- test-1 still present, now via the pattern ---"
ppz_a subs ls | ls_normalize
