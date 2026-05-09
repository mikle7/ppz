#!/usr/bin/env bash
# Reliability suite runner. Wraps tests/run.sh with the `reliability/`
# filter so we only execute scenarios under tests/reliability/* — the
# assumption being that the stack has been brought up via
# `make e2e-reliability{,-up}` with the reliability overlay (Docker
# socket mounted on test-runner so scenarios can stop / start
# sibling containers).
#
# Run from inside the test-runner container the same way the regular
# run.sh runs.
#
# Reliability scenarios may pause / restart `ppz-server` to simulate
# outages. To keep scenarios independent we ensure ppz-server is
# running before each one — `docker start` is idempotent.

set -u
set -o pipefail

TESTS_DIR="$(cd "$(dirname "$0")/.." && pwd)"
LIB_DIR="$TESTS_DIR/lib"
# shellcheck source=../lib/common.sh
. "$LIB_DIR/common.sh"

PASS=0
FAIL=0
FAILED_SCENARIOS=()

filter="${PPZ_TEST_FILTER:-reliability/*}"
case "$filter" in
  reliability/*) ;;
  *) filter="reliability/$filter" ;;
esac

export PPZ_DIAG_LOG=/tmp/ppz-diag.log
: > "$PPZ_DIAG_LOG"

# Sanity: the Docker socket has to be reachable, otherwise reliability
# scenarios that issue `docker stop`/`docker start` will all fail with
# the same opaque error. Surface that early.
if ! docker info >/dev/null 2>&1; then
  echo "FAIL reliability suite (Docker socket not reachable from test-runner)"
  echo "    expected /var/run/docker.sock to be mounted via compose/docker-compose.reliability.yml"
  echo "    run: make e2e-reliability  (not e2e)"
  exit 1
fi

mapfile -t scenarios < <(find "$TESTS_DIR/reliability" -mindepth 2 -maxdepth 3 -name run.sh 2>/dev/null \
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

  # Reliability scenarios may have stopped ppz-server in a previous
  # run. `docker start` is idempotent (no-op when already running) so
  # this just ensures we begin from a known-up state.
  docker start compose-ppz-server-1 >/dev/null 2>&1 || true
  # Wait for the server's HTTP healthcheck to pass before continuing —
  # otherwise reset.sh below races against the postgres / NATS subsystems
  # still spinning up and produces unhelpful failures.
  for i in $(seq 1 30); do
    if curl -sf http://ppz-server:8080/healthz >/dev/null 2>&1; then break; fi
    sleep 1
  done

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
  # Reliability scenarios can take much longer than standard or WAN
  # because they exercise NATS outage recovery (the recovers-from-long-
  # outage scenario alone is >150s). 300s ceiling — broken baselines
  # still finish so we observe the real wall-time and diff a clean
  # assertion miss instead of a timeout.
  timeout 300s bash "$dir/run.sh" >"$actual" 2>/dev/null || rc=$?
  end_ts=$(date -u +%s.%N 2>/dev/null || date -u +%s)
  elapsed=$(awk -v a="$end_ts" -v b="$start_ts" 'BEGIN{printf "%.1f", a-b}' 2>/dev/null || echo "?")
  echo "exit=$rc" >>"$actual"

  if [[ $rc -eq 124 || $rc -eq 143 ]]; then
    echo "FAIL $rel (timeout: exceeded 300s)"
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

# Best-effort: leave ppz-server running so subsequent suites or manual
# debugging don't trip over a stopped server.
docker start compose-ppz-server-1 >/dev/null 2>&1 || true

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
