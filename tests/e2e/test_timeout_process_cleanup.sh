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
  local runner_state=""
  local runner_status=0
  local attempt

  for attempt in {1..120}; do
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

run_adversarial_detector_check
run_timeout_cleanup_check
echo "PASS: BUG-031-004 timeout process cleanup regression"
