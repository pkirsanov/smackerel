#!/usr/bin/env bash
# E2E Test Runner — boots the stack once, runs all shared-stack tests, then
# runs lifecycle tests that manage their own stack.
#
# Usage:
#   bash tests/e2e/run_all.sh              # run all tests
#   bash tests/e2e/run_all.sh test_search* # run matching tests only
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PATTERN="${1:-test_*.sh}"
TEST_ENV="${TEST_ENV:-test}"
source "$SCRIPT_DIR/lib/helpers.sh"

PASSED=0
FAILED=0
RESULTS=()

# Lifecycle tests manage their own stack boot/teardown and must run standalone.
LIFECYCLE_TESTS="test_compose_start test_persistence test_postgres_readiness_gate test_config_fail"

is_lifecycle_test() {
  local name="$1"
  for lt in $LIFECYCLE_TESTS; do
    [[ "$name" == "$lt" ]] && return 0
  done
  return 1
}

run_test() {
  local test_file="$1"
  local test_name
  test_name="$(basename "$test_file" .sh)"

  echo "--- Running: $test_name ---"
  set +e
  bash "$test_file" 2>&1
  local exit_code=$?
  set -e

  if [ $exit_code -eq 0 ]; then
    RESULTS+=("PASS: $test_name")
    PASSED=$((PASSED + 1))
  else
    RESULTS+=("FAIL: $test_name (exit=$exit_code)")
    FAILED=$((FAILED + 1))
  fi
  echo ""
}

echo "========================================="
echo "  Smackerel E2E Test Suite"
echo "========================================="
echo ""

# ── Phase 1: Shared-stack tests ──────────────────────────────────────────────
# Boot the test stack once, run all standard tests against it, then tear down.

SHARED_TESTS=()
LIFECYCLE_TEST_FILES=()

for TEST_FILE in "$SCRIPT_DIR"/$PATTERN; do
  [ -f "$TEST_FILE" ] || continue
  TEST_NAME="$(basename "$TEST_FILE" .sh)"
  [[ "$TEST_NAME" == "run_all" ]] && continue

  if is_lifecycle_test "$TEST_NAME"; then
    LIFECYCLE_TEST_FILES+=("$TEST_FILE")
  else
    SHARED_TESTS+=("$TEST_FILE")
  fi
done

if [ ${#SHARED_TESTS[@]} -gt 0 ]; then
  echo "== Phase 1: Shared-stack tests (${#SHARED_TESTS[@]} tests) =="
  echo "Booting test stack..."
  "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" down --volumes >/dev/null 2>&1 || true
  "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" up

  e2e_setup
  e2e_wait_healthy 120

  echo ""

  for TEST_FILE in "${SHARED_TESTS[@]}"; do
    E2E_STACK_MANAGED=1 run_test "$TEST_FILE"
  done

  echo "Tearing down shared test stack..."
  "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" down --volumes >/dev/null 2>&1 || true
  echo ""
fi

# ── Phase 2: Lifecycle tests ─────────────────────────────────────────────────
# These tests boot/stop/restart the stack themselves.

if [ ${#LIFECYCLE_TEST_FILES[@]} -gt 0 ]; then
  echo "== Phase 2: Lifecycle tests (${#LIFECYCLE_TEST_FILES[@]} tests) =="
  for TEST_FILE in "${LIFECYCLE_TEST_FILES[@]}"; do
    run_test "$TEST_FILE"
  done
fi

# ── Results ──────────────────────────────────────────────────────────────────

TOTAL=$((PASSED + FAILED))
echo "========================================="
echo "  E2E Test Results"
echo "========================================="
for R in "${RESULTS[@]}"; do
  echo "  $R"
done
echo ""
echo "  Total:  $TOTAL"
echo "  Passed: $PASSED"
echo "  Failed: $FAILED"
echo "========================================="

if [ $FAILED -gt 0 ]; then
  exit 1
fi
