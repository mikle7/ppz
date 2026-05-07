#!/usr/bin/env bash
# WAN suite runner. Wraps tests/run.sh with the `wan/` filter so we
# only execute scenarios under tests/wan/* — the assumption being
# that the stack has been brought up via `make e2e-wan{,-up}` with
# the latency overlay (daemon-a egress under tc netem).
#
# Run from inside the test-runner container the same way the regular
# run.sh runs.

set -u
set -o pipefail

# tests/run.sh skips wan/ by default; passing it back as a filter via
# env requires a flag the harness supports.  Easiest: run the whole
# loop here, scoped to wan/*.
TESTS_DIR="$(cd "$(dirname "$0")/.." && pwd)"
LIB_DIR="$TESTS_DIR/lib"
# shellcheck source=../lib/common.sh
. "$LIB_DIR/common.sh"

PASS=0
FAIL=0
FAILED_SCENARIOS=()

filter="${PPZ_TEST_FILTER:-wan/*}"
case "$filter" in
  wan/*) ;;
  *) filter="wan/$filter" ;;
esac

export PPZ_DIAG_LOG=/tmp/ppz-diag.log
: > "$PPZ_DIAG_LOG"

mapfile -t scenarios < <(find "$TESTS_DIR/wan" -mindepth 2 -maxdepth 3 -name run.sh 2>/dev/null \
  | while read -r f; do dirname "$f"; done | sort)

for dir in "${scenarios[@]}"; do
  rel="${dir#$TESTS_DIR/}"
  # shellcheck disable=SC2053
  if [[ "$rel" != $filter && "$rel" != $filter/* ]]; then
    continue
  fi
  if [[ ! -f "$dir/expected.txt" ]]; then
    echo "SKIP $rel (no expected.txt)"
    continue
  fi

  reset_err="$(mktemp)"
  if ! SCENARIO="$rel" bash "$LIB_DIR/reset.sh" >/dev/null 2>"$reset_err"; then
    echo "FAIL $rel (reset.sh failed)"
    sed 's/^/    /' "$reset_err"
    FAIL=$((FAIL + 1))
    FAILED_SCENARIOS+=("$rel (reset.sh)")
    rm -f "$reset_err"
    continue
  fi
  rm -f "$reset_err"

  actual="$(mktemp)"
  normalized="$(mktemp)"
  rc=0
  start_ts=$(date -u +%s.%N 2>/dev/null || date -u +%s)
  # WAN scenarios are expected to take longer (latency * many calls).
  # 120s ceiling, vs 30s for the standard suite — gives broken
  # baselines room to finish so we can observe the real wall-time
  # and diff a clean assertion miss instead of a timeout.
  timeout 120s bash "$dir/run.sh" >"$actual" 2>/dev/null || rc=$?
  end_ts=$(date -u +%s.%N 2>/dev/null || date -u +%s)
  elapsed=$(awk -v a="$end_ts" -v b="$start_ts" 'BEGIN{printf "%.1f", a-b}' 2>/dev/null || echo "?")
  echo "exit=$rc" >>"$actual"

  if [[ $rc -eq 124 || $rc -eq 143 ]]; then
    echo "FAIL $rel (timeout: exceeded 120s)"
    FAIL=$((FAIL + 1))
    FAILED_SCENARIOS+=("$rel (timeout)")
    rm -f "$actual" "$normalized"
    continue
  fi

  bash "$LIB_DIR/normalize.sh" <"$actual" >"$normalized"

  if diff -u "$dir/expected.txt" "$normalized" >/dev/null; then
    echo "PASS $rel (${elapsed}s)"
    PASS=$((PASS + 1))
  else
    echo "FAIL $rel (${elapsed}s)"
    diff -u "$dir/expected.txt" "$normalized" | sed 's/^/    /'
    FAIL=$((FAIL + 1))
    FAILED_SCENARIOS+=("$rel")
  fi

  rm -f "$actual" "$normalized"
done

TOTAL=$((PASS + FAIL))
echo
if [[ $FAIL -eq 0 ]]; then
  echo "PASS: $PASS/$TOTAL"
  exit 0
else
  echo "FAIL: $FAIL/$TOTAL"
  printf '  - %s\n' "${FAILED_SCENARIOS[@]}"
  exit 1
fi
