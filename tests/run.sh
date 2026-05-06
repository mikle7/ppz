#!/usr/bin/env bash
# ppz end-to-end test harness.
#
# Iterates every directory under tests/ that contains a run.sh + expected.txt.
# For each scenario:
#   1. Resets shared state via tests/lib/reset.sh
#   2. Runs run.sh, capturing stdout (stderr is discarded — assertions are
#      against stdout only; error tests assert exit code).
#   3. Pipes stdout through tests/lib/normalize.sh (replaces pids, uuids,
#      timestamps, key prefixes, seeded org IDs with stable tokens).
#   4. Diffs the normalized output against expected.txt.
#
# Honours PPZ_TEST_FILTER (glob) to run a subset.
# Exits 0 only if every scenario passes; non-zero on any FAIL.

set -u
set -o pipefail

TESTS_DIR="$(cd "$(dirname "$0")" && pwd)"
LIB_DIR="$TESTS_DIR/lib"

# shellcheck source=lib/common.sh
. "$LIB_DIR/common.sh"

PASS=0
FAIL=0
FAILED_SCENARIOS=()

filter="${PPZ_TEST_FILTER:-*}"

# Per-run diagnostic log. reset.sh appends one block per scenario.
# On FAIL, run.sh dumps the most recent few blocks alongside the diff
# so flaky cross-scenario state shows up in the test output.
export PPZ_DIAG_LOG=/tmp/ppz-diag.log
: > "$PPZ_DIAG_LOG"

# Collect scenarios deterministically.
mapfile -t scenarios < <(find "$TESTS_DIR" -mindepth 2 -maxdepth 3 -name run.sh | while read -r f; do dirname "$f"; done | sort)

for dir in "${scenarios[@]}"; do
  rel="${dir#$TESTS_DIR/}"
  # WAN scenarios run only under the e2e-wan harness (tests/wan/run.sh),
  # which boots compose with the latency overlay. They'd false-pass
  # here (no latency injected) — skip outright.
  case "$rel" in
    wan/*) continue ;;
  esac
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
    # reset failure means cross-scenario state leaked. Treat the
    # scenario as failed and surface the reason — don't silently
    # plow on with a broken environment.
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
  # 30s ceiling per scenario. Exit codes: 124 (GNU coreutils) or 143
  # (BusyBox: 128+SIGTERM) both mean the cap fired. We branch on either
  # so the FAIL message is clean instead of a misleading diff.
  timeout 30s bash "$dir/run.sh" >"$actual" 2>/dev/null || rc=$?
  end_ts=$(date -u +%s.%N 2>/dev/null || date -u +%s)
  elapsed=$(awk -v a="$end_ts" -v b="$start_ts" 'BEGIN{printf "%.1f", a-b}' 2>/dev/null || echo "?")
  echo "exit=$rc" >>"$actual"

  if [[ $rc -eq 124 || $rc -eq 143 ]]; then
    echo "FAIL $rel (timeout: exceeded 30s)"
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
    # Dump live diagnostic state so flake post-mortems have evidence:
    # - last few reset.sh blocks (prior-scenario leakage)
    # - current JetStream stream list (anything we left behind)
    # - daemon credentials shape (truncated jwt, missing fields, …)
    echo "    ── diag ──────────────────────────────────────────────────"
    if [[ -f "$PPZ_DIAG_LOG" ]]; then
      tail -30 "$PPZ_DIAG_LOG" | sed 's/^/    diag| /'
    fi
    if [[ -f /seed/nats-server-user.creds ]] && command -v nats >/dev/null 2>&1; then
      streams=$(nats --server=nats://ppz-server:4222 \
                     --creds=/seed/nats-server-user.creds \
                     stream ls -n 2>&1 | head -20)
      echo "    streams| $(echo "$streams" | wc -l) line(s):"
      echo "$streams" | sed 's/^/    streams|   /'
    fi
    for h in "${PPZ_DAEMON_A_HOME:-/tmp/a}" "${PPZ_DAEMON_B_HOME:-/tmp/b}"; do
      if [[ -f "$h/credentials" ]]; then
        size=$(wc -c < "$h/credentials")
        keys=$(grep -oE '"[a-z_]+"' "$h/credentials" | sort -u | tr '\n' ' ')
        echo "    daemon| $h/credentials size=${size}B keys=$keys"
      fi
    done
    echo "    ──────────────────────────────────────────────────────────"
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
