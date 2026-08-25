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
SKIPPED=0
REQUIRED_SKIPPED=0
RESULTS=()

# A fixture can prove a behaviour, disprove it, or not run. Exit 77 is this
# repository's shell convention for the third outcome; see
# tests/e2e/assistant_regression/lib/regression_helpers.sh::reg_skip_with_blocker.
SKIP_EXIT_CODE=77

# One run-scoped capture file rather than one per fixture. tee truncates on
# open, so fixtures cannot see each other's output.
RUN_OUTPUT_FILE="$(mktemp)"
cleanup_run_output() { rm -f "$RUN_OUTPUT_FILE"; }
# EXIT alone is not enough: bash does not run it when killed by an untrapped
# SIGTERM, which is how the `timeout` wrapper in the CLI lane stops this runner.
# INT and TERM are handled explicitly so the conventional exit codes survive.
trap cleanup_run_output EXIT
trap 'cleanup_run_output; exit 130' INT
trap 'cleanup_run_output; exit 143' TERM

# Lifecycle tests manage their own stack boot/teardown and must run standalone.
LIFECYCLE_TESTS="test_timeout_process_cleanup test_compose_start test_persistence test_postgres_readiness_gate test_config_fail"

# Fixtures whose skip must keep the suite non-green. Requiredness is declared
# here by the runner, in the same explicit-array idiom as LIFECYCLE_TESTS, so a
# fixture cannot downgrade itself out of the required set by editing its own
# body. A required fixture that skips is still reported as SKIP — the label
# stays honest — and the suite exit stays non-zero because the behaviour the
# fixture covers is unproven.
REQUIRED_TESTS="test_timeout_process_cleanup test_deploy_target_status test_compose_start test_persistence test_postgres_readiness_gate test_config_fail test_capture_pipeline test_voice_pipeline test_llm_failure_e2e test_capture_api test_capture_errors test_voice_capture_api test_knowledge_graph test_graph_entities test_search test_search_filters test_search_empty test_telegram test_telegram_auth test_telegram_voice test_telegram_format test_digest test_digest_quiet test_digest_telegram test_web_ui test_web_detail test_web_settings test_connector_framework test_imap_sync test_caldav_sync test_youtube_sync test_bookmark_import test_topic_lifecycle test_settings_connectors test_maps_import test_browser_sync"

is_lifecycle_test() {
  local name="$1"
  for lt in $LIFECYCLE_TESTS; do
    [[ "$name" == "$lt" ]] && return 0
  done
  return 1
}

is_required_test() {
  local name="$1"
  for rt in $REQUIRED_TESTS; do
    [[ "$name" == "$rt" ]] && return 0
  done
  return 1
}

run_test() {
  local test_file="$1"
  local test_name exit_code reason
  test_name="$(basename "$test_file" .sh)"

  echo "--- Running: $test_name ---"
  set +e
  # tee keeps the fixture's own output streaming live while giving the
  # classifier a copy to read SKIP_REASON from; nothing is swallowed.
  bash "$test_file" 2>&1 | tee "$RUN_OUTPUT_FILE"
  exit_code=${PIPESTATUS[0]}
  set -e

  if [ "$exit_code" -eq 0 ]; then
    RESULTS+=("PASS: $test_name")
    PASSED=$((PASSED + 1))
  elif [ "$exit_code" -eq "$SKIP_EXIT_CODE" ]; then
    reason="$(sed -n 's/^SKIP_REASON:[[:space:]]*//p' "$RUN_OUTPUT_FILE" | head -1)"
    [ -n "$reason" ] || reason="no SKIP_REASON emitted by $test_file"
    SKIPPED=$((SKIPPED + 1))
    if is_required_test "$test_name"; then
      RESULTS+=("SKIP: $test_name ($reason) [required]")
      REQUIRED_SKIPPED=$((REQUIRED_SKIPPED + 1))
    else
      RESULTS+=("SKIP: $test_name ($reason)")
    fi
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

TOTAL=$((PASSED + FAILED + SKIPPED))
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
echo "  Skipped: $SKIPPED"
if [ $REQUIRED_SKIPPED -gt 0 ]; then
  echo "  Required skips: $REQUIRED_SKIPPED (behaviour declared required is unproven)"
fi
echo "========================================="

if [ $FAILED -gt 0 ] || [ $REQUIRED_SKIPPED -gt 0 ]; then
  exit 1
fi
