#!/usr/bin/env bash
# `subs rm` keeps its exact-string-match, idempotent semantics UNCHANGED
# (no exclusions, no pattern subtraction) — it just gains feedback so a
# no-op is never silent. Three cases, all exit 0 (idempotent rm, matching
# subs-add-rm-idempotent):
#   (a) removing a stored subject confirms what went;
#   (b) removing an expanded MATCH of a pattern (not itself a stored
#       subject) removes nothing and explains it's covered by the pattern —
#       directly addressing the "I rm'd the row I can see and it came back"
#       confusion;
#   (c) removing a never-subscribed subject removes nothing and says so.
. /tests/lib/common.sh
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1
ppz_a pipe create test-1 >/dev/null
export PPZ_SESSION=mysh
ppz_a subs add 'test-%' >/dev/null

echo "--- (a) rm a stored pattern -> confirms removal ---"
ppz_a subs rm 'test-%'; echo "rc=$?"

echo "--- (b) rm an expanded match (not stored) -> no-op + pattern hint ---"
ppz_a subs add 'test-%' >/dev/null
ppz_a subs rm test-1; echo "rc=$?"

echo "--- pattern survives (b) ---"
ppz_a subs ls | ls_normalize | grep -c '^test-%' | sed 's/^/pattern-rows=/'

echo "--- (c) rm a never-subscribed subject -> no-op + says so ---"
ppz_a subs rm nonexistent; echo "rc=$?"
