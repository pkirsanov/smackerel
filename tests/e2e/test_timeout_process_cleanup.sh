#!/usr/bin/env bash
# E2E regression: BUG-031-004 timeout cleanup terminates stubborn child work.
# Scenarios: BUG-031-004-SCN-001, BUG-031-004-SCN-002
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
FIXTURE="$SCRIPT_DIR/fixtures/test_timeout_child_fixture.sh"
BASE_MARKER="smackerel-e2e-timeout-cleanup-$$-$(date +%s)"
RUNNER_LOG=""
RUNNER_PID=""
ADVERSARIAL_PARENT_PID=""
DOCKER_RUNNER_ID=""
DOCKER_CANARY_ID=""

marker_pids() {
  local marker="$1"
  local process_id command_args

  while read -r process_id command_args; do
    if [[ -n "${process_id:-}" && "${command_args:-}" == *"$marker"* ]]; then
      printf '%s\n' "$process_id"
    fi
  done < <(ps -eo pid=,args=)
}

cleanup_marker_processes() {
  local marker="$1"
  local marker_processes=()

  mapfile -t marker_processes < <(marker_pids "$marker")
  if [[ ${#marker_processes[@]} -gt 0 ]]; then
    kill -KILL "${marker_processes[@]}" 2>/dev/null || true
  fi
}

cleanup() {
  if [[ -n "${RUNNER_PID:-}" ]] && kill -0 "$RUNNER_PID" 2>/dev/null; then
    kill -KILL "$RUNNER_PID" 2>/dev/null || true
    wait "$RUNNER_PID" 2>/dev/null || true
  fi
  if [[ -n "${ADVERSARIAL_PARENT_PID:-}" ]] && kill -0 "$ADVERSARIAL_PARENT_PID" 2>/dev/null; then
    kill -KILL "$ADVERSARIAL_PARENT_PID" 2>/dev/null || true
    wait "$ADVERSARIAL_PARENT_PID" 2>/dev/null || true
  fi
  cleanup_marker_processes "${BASE_MARKER}-adversarial"
  cleanup_marker_processes "${BASE_MARKER}-runner"
  if [[ -n "${DOCKER_RUNNER_ID:-}" ]]; then
    docker rm --force "$DOCKER_RUNNER_ID" >/dev/null 2>&1 || true
  fi
  if [[ -n "${DOCKER_CANARY_ID:-}" ]]; then
    docker rm --force "$DOCKER_CANARY_ID" >/dev/null 2>&1 || true
  fi
  "$REPO_DIR/smackerel.sh" --env test down --volumes >/dev/null 2>&1 || true
  if [[ -n "${RUNNER_LOG:-}" && -f "$RUNNER_LOG" ]]; then
    rm -f "$RUNNER_LOG"
  fi
}
trap cleanup EXIT

e2e_fail() {
  echo "FAIL: $1"
  if [[ -n "${RUNNER_LOG:-}" && -f "$RUNNER_LOG" ]]; then
    echo "Nested runner log:"
    cat "$RUNNER_LOG"
  fi
  exit 1
}

wait_for_marker_process() {
  local marker="$1"
  local marker_processes=()
  local attempt

  for attempt in {1..80}; do
    mapfile -t marker_processes < <(marker_pids "$marker")
    if [[ ${#marker_processes[@]} -gt 0 ]]; then
      echo "Observed marker process for $marker: ${marker_processes[*]}"
      return 0
    fi
    sleep 0.25
  done

  return 1
}

assert_no_marker_processes() {
  local marker="$1"
  local marker_processes=()

  mapfile -t marker_processes < <(marker_pids "$marker")
  if [[ ${#marker_processes[@]} -gt 0 ]]; then
    echo "Surviving child work for marker $marker: ${marker_processes[*]}" >&2
    return 1
  fi
}

wait_for_no_marker_processes() {
  local marker="$1"
  local attempt

  for attempt in {1..80}; do
    if assert_no_marker_processes "$marker" >/dev/null 2>&1; then
      echo "Marker processes absent for $marker"
      return 0
    fi
    sleep 0.25
  done

  assert_no_marker_processes "$marker"
}

wait_for_runner_exit() {
  local runner_pid="$1"
  local max_attempts="${2:-120}"
  local runner_state=""
  local runner_status=0
  local attempt

  for ((attempt = 0; attempt < max_attempts; attempt++)); do
    runner_state="$(ps -p "$runner_pid" -o stat= 2>/dev/null || true)"
    if [[ -z "$runner_state" || "$runner_state" == Z* ]]; then
      set +e
      wait "$runner_pid"
      runner_status=$?
      set -e
      printf '%s\n' "$runner_status"
      return 0
    fi
    sleep 0.25
  done

  echo "Nested E2E runner did not exit after interruption" >&2
  return 1
}

wait_for_log_line() {
  local expected="$1"
  local attempt

  for attempt in {1..480}; do
    if [[ -n "${RUNNER_LOG:-}" && -f "$RUNNER_LOG" ]] && grep -Fq "$expected" "$RUNNER_LOG"; then
      echo "Observed nested runner log marker: $expected"
      return 0
    fi
    sleep 0.25
  done

  return 1
}

wait_for_go_runner_container() {
  local runner_ids=()
  local attempt

  for attempt in {1..480}; do
    mapfile -t runner_ids < <(docker ps -q \
      --filter "ancestor=golang:1.25.10-bookworm" \
      --filter "network=smackerel-test_default")
    if [[ ${#runner_ids[@]} -eq 1 ]]; then
      DOCKER_RUNNER_ID="${runner_ids[0]}"
      echo "Observed Go E2E runner container: $DOCKER_RUNNER_ID"
      return 0
    fi
    if [[ ${#runner_ids[@]} -gt 1 ]]; then
      echo "Expected one Go E2E runner container, found ${#runner_ids[@]}: ${runner_ids[*]}" >&2
      return 1
    fi
    sleep 0.25
  done

  return 1
}

container_is_running() {
  local container_id="$1"
  [[ "$(docker inspect --format '{{.State.Running}}' "$container_id" 2>/dev/null || true)" == "true" ]]
}

container_run_id() {
  local container_id="$1"
  docker inspect --format '{{index .Config.Labels "com.smackerel.e2e-child-run-id"}}' "$container_id" 2>/dev/null || true
}

run_adversarial_detector_check() {
  local marker="${BASE_MARKER}-adversarial"
  local detector_output=""
  local detector_status=0

  echo "=== BUG-031-004-SCN-002: regression detects surviving child work ==="
  SMACKEREL_E2E_LEAK_MARKER="$marker" bash "$FIXTURE" &
  ADVERSARIAL_PARENT_PID=$!

  wait_for_marker_process "$marker" || e2e_fail "adversarial fixture did not create a marker process"

  set +e
  detector_output="$(assert_no_marker_processes "$marker" 2>&1)"
  detector_status=$?
  set -e

  if [[ "$detector_status" -eq 0 ]]; then
    e2e_fail "surviving-child detector passed despite an active marker process"
  fi
  if [[ "$detector_output" != *"$marker"* ]]; then
    e2e_fail "surviving-child detector did not report the marker name"
  fi

  echo "Detector reported surviving child work: $detector_output"
  kill -KILL "$ADVERSARIAL_PARENT_PID" 2>/dev/null || true
  wait "$ADVERSARIAL_PARENT_PID" 2>/dev/null || true
  ADVERSARIAL_PARENT_PID=""
  cleanup_marker_processes "$marker"
  wait_for_no_marker_processes "$marker" || e2e_fail "adversarial marker process survived cleanup"
  echo "PASS: BUG-031-004-SCN-002"
}

run_timeout_cleanup_check() {
  local marker="${BASE_MARKER}-runner"
  local runner_status=""

  echo "=== BUG-031-004-SCN-001: E2E interruption terminates child processes ==="
  RUNNER_LOG="$(mktemp)"
  SMACKEREL_E2E_LEAK_MARKER="$marker" "$REPO_DIR/smackerel.sh" --env test test e2e --shell-run fixtures/test_timeout_child_fixture.sh >"$RUNNER_LOG" 2>&1 &
  RUNNER_PID=$!

  wait_for_marker_process "$marker" || e2e_fail "nested E2E runner did not start the adversarial child fixture"

  echo "Interrupting nested E2E runner pid $RUNNER_PID"
  kill -TERM "$RUNNER_PID"

  runner_status="$(wait_for_runner_exit "$RUNNER_PID")" || e2e_fail "nested E2E runner failed to exit after interruption"
  RUNNER_PID=""

  if [[ "$runner_status" -eq 0 ]]; then
    e2e_fail "nested E2E runner returned success after TERM interruption"
  fi
  echo "Nested E2E runner returned nonzero after interruption: $runner_status"
  if ! grep -q "Running project-scoped test stack teardown (exit cleanup" "$RUNNER_LOG"; then
    e2e_fail "nested E2E runner did not execute project-scoped test-stack cleanup"
  fi

  wait_for_no_marker_processes "$marker" || e2e_fail "marker child process survived E2E timeout cleanup"
  echo "PASS: BUG-031-004-SCN-001"
}

run_docker_runner_cleanup_check() {
  local runner_status=""
  local canary_label="smackerel-e2e-canary-${BASE_MARKER}"

  echo "=== BUG-031-009-SCN-001/002: interrupted Docker Go runner is reaped before teardown ==="
  RUNNER_LOG="$(mktemp)"
  DOCKER_CANARY_ID="$(docker run -d --rm \
    --label "com.smackerel.e2e-child-run-id=$canary_label" \
    golang:1.25.10-bookworm tail -f /dev/null)"
  if [[ -z "$DOCKER_CANARY_ID" ]]; then
    e2e_fail "failed to start nonmatching Docker cleanup canary"
  fi

  SMACKEREL_HARDWARE_TIER=cpu "$REPO_DIR/smackerel.sh" --env test test e2e \
    --go-run '^(TestDrive|TestMultiProviderDrive|TestLowConfidenceConfirmation|TestTelegramRetrieval|TestFolderMove|TestSkippedAndBlocked|TestSaveRulesList|TestTelegramReceipt)' \
    >"$RUNNER_LOG" 2>&1 &
  RUNNER_PID=$!

  wait_for_go_runner_container || e2e_fail "nested E2E did not start exactly one Go runner container"
  wait_for_log_line "=== RUN   TestDrive" || e2e_fail "nested Go runner did not begin a Drive test"
  if [[ "$(container_run_id "$DOCKER_RUNNER_ID")" != smackerel-e2e-child-* ]]; then
    e2e_fail "Dockerized Go E2E runner $DOCKER_RUNNER_ID has no per-run cleanup identity"
  fi

  echo "Interrupting nested Dockerized E2E runner pid $RUNNER_PID"
  kill -TERM "$RUNNER_PID"
  wait_for_log_line "Running project-scoped test stack teardown (exit cleanup" || e2e_fail "nested E2E did not begin stack teardown after interruption"

  if container_is_running "$DOCKER_RUNNER_ID"; then
    e2e_fail "Dockerized Go E2E runner $DOCKER_RUNNER_ID survived until stack teardown began"
  fi
  if ! container_is_running "$DOCKER_CANARY_ID"; then
    e2e_fail "nonmatching Docker canary $DOCKER_CANARY_ID was removed by interrupted-run cleanup"
  fi

  runner_status="$(wait_for_runner_exit "$RUNNER_PID" 800)" || e2e_fail "nested Dockerized E2E runner failed to exit after interruption"
  RUNNER_PID=""
  if [[ "$runner_status" -eq 0 ]]; then
    e2e_fail "nested Dockerized E2E runner returned success after TERM interruption"
  fi

  docker rm --force "$DOCKER_CANARY_ID" >/dev/null
  DOCKER_CANARY_ID=""
  DOCKER_RUNNER_ID=""
  echo "PASS: BUG-031-009-SCN-001"
  echo "PASS: BUG-031-009-SCN-002"
}

run_adversarial_detector_check
run_timeout_cleanup_check
run_docker_runner_cleanup_check
echo "PASS: BUG-031-004 timeout process cleanup regression"
