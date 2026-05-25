#!/usr/bin/env bash
set -euo pipefail

# convergence-cap-guard-selftest.sh
#
# Hermetic selftest for `bubbles/scripts/convergence-cap-guard.sh`
# (Gate G082 — convergence_cap_enforcement_gate).
#
# Builds a private mktemp Bubbles-repo surface (no edits to the host
# repo), stages three fixture scenarios in its `.specify/memory/`
# directory, invokes the guard with explicit `BUBBLES_REPO_ROOT`, and
# asserts exit codes plus stdout/stderr fingerprints.
#
# Scenarios (matches scope.md Gherkin):
#   S1: iterationCount = 11  → exit 1, stderr contains "G082" and
#                              "maxConvergenceIterations"
#   S2: iterationCount = 10  → exit 0, stdout contains "PASS"
#   S3: malformed session.json → exit 2, stderr contains diagnostic
#
# Additionally:
#   S0: empty / missing convergenceLoops[] → exit 0 (sanity check
#       proving the guard is no-op for specs that have not yet looped)
#
# Reference:
#   docs/Framework_Convergence_Health.md

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd -P)"
GUARD_SCRIPT="$SCRIPT_DIR/convergence-cap-guard.sh"

if [[ ! -x "$GUARD_SCRIPT" ]]; then
  echo "convergence-cap-guard-selftest: guard script not executable: $GUARD_SCRIPT" >&2
  exit 2
fi

# --- Hermetic workspace --------------------------------------------------

WORKSPACE="$(mktemp -d -t bubbles-conv-cap-selftest-XXXXXXXX)"
cleanup() {
  rm -rf "$WORKSPACE"
}
trap cleanup EXIT INT TERM

PASS_COUNT=0
FAIL_COUNT=0
declare -a FAILED_SCENARIOS=()

note() { printf '[selftest] %s\n' "$*"; }
ok()   { printf '[selftest] PASS: %s\n' "$*"; PASS_COUNT=$((PASS_COUNT + 1)); }
ko()   {
  printf '[selftest] FAIL: %s\n' "$*" >&2
  FAIL_COUNT=$((FAIL_COUNT + 1))
  FAILED_SCENARIOS+=("$1")
}

# --- Stage a minimal fake "Bubbles" repo surface inside WORKSPACE --------
#
# We need:
#   <root>/.specify/memory/bubbles.session.json
#   <root>/bubbles/workflows.yaml   (with maxConvergenceIterations: 10)
#
# The selftest stages files INSIDE its own mktemp workspace via heredocs.
# This is allowed by terminal-discipline policy (heredoc-to-file is
# forbidden for repo files; the workspace here is throwaway and never
# becomes part of the working tree).

stage_repo_root() {
  local root="$1"
  local cap="${2:-10}"

  mkdir -p "$root/.specify/memory" "$root/bubbles"

  cat > "$root/bubbles/workflows.yaml" <<EOF
# Minimal workflows.yaml fixture for convergence-cap-guard selftest.
workflowModes:
  autonomous-goal:
    constraints:
      maxConvergenceIterations: $cap
EOF
}

write_session_json() {
  local root="$1"
  local payload="$2"

  printf '%s\n' "$payload" > "$root/.specify/memory/bubbles.session.json"
}

# --- Helper: run guard, capture exit + stdout + stderr -------------------

run_guard() {
  local root="$1"
  local spec_dir="$2"
  local stdout_file="$WORKSPACE/stdout.last"
  local stderr_file="$WORKSPACE/stderr.last"

  set +e
  BUBBLES_REPO_ROOT="$root" bash "$GUARD_SCRIPT" "$spec_dir" \
    > "$stdout_file" \
    2> "$stderr_file"
  local rc=$?
  set -e

  printf '%s\n' "$rc" > "$WORKSPACE/exit.last"
  printf '%s' "$stdout_file"
}

last_exit()   { cat "$WORKSPACE/exit.last"; }
last_stdout() { cat "$WORKSPACE/stdout.last"; }
last_stderr() { cat "$WORKSPACE/stderr.last"; }

assert_exit() {
  local expected="$1"
  local label="$2"
  local actual
  actual="$(last_exit)"
  if [[ "$actual" != "$expected" ]]; then
    ko "$label: expected exit $expected, got $actual"
    echo "  --- stdout ---" >&2
    last_stdout >&2
    echo "  --- stderr ---" >&2
    last_stderr >&2
    return 1
  fi
  ok "$label: exit $expected"
}

assert_stdout_contains() {
  local needle="$1"
  local label="$2"
  if ! grep -Fq -- "$needle" "$WORKSPACE/stdout.last"; then
    ko "$label: stdout did not contain '$needle'"
    echo "  --- stdout ---" >&2
    last_stdout >&2
    return 1
  fi
  ok "$label: stdout contains '$needle'"
}

assert_stderr_contains() {
  local needle="$1"
  local label="$2"
  if ! grep -Fq -- "$needle" "$WORKSPACE/stderr.last"; then
    ko "$label: stderr did not contain '$needle'"
    echo "  --- stderr ---" >&2
    last_stderr >&2
    return 1
  fi
  ok "$label: stderr contains '$needle'"
}

# =============================================================================
# Scenario S0: empty convergenceLoops[] -> exit 0 (sanity check)
# =============================================================================

note "Scenario S0: empty convergenceLoops[] should pass with exit 0"

S0_ROOT="$WORKSPACE/s0"
stage_repo_root "$S0_ROOT" 10
write_session_json "$S0_ROOT" '{"convergenceLoops": []}'

run_guard "$S0_ROOT" "specs/900-convergence-fixture" >/dev/null

assert_exit 0 "S0 exit code"
assert_stdout_contains "PASS Gate G082" "S0 PASS marker on stdout"
assert_stdout_contains "observed=0" "S0 reports zero observed iterations"

# =============================================================================
# Scenario S1: iterationCount = 11 -> exit 1, stderr names G082 + cap
# =============================================================================

note "Scenario S1: iterationCount=11 above cap=10 should exit 1"

S1_ROOT="$WORKSPACE/s1"
stage_repo_root "$S1_ROOT" 10
write_session_json "$S1_ROOT" '{
  "convergenceLoops": [
    {
      "specDir": "specs/900-convergence-fixture",
      "agent": "bubbles.workflow",
      "iterationCount": 11,
      "lastIterationAt": "2026-06-01T10:00:00Z",
      "cappedAt": null
    }
  ]
}'

run_guard "$S1_ROOT" "specs/900-convergence-fixture" >/dev/null

assert_exit 1 "S1 exit code (cap exceeded)"
assert_stderr_contains "G082" "S1 stderr names Gate G082"
assert_stderr_contains "convergence_cap_enforcement_gate" "S1 stderr names gate full name"
assert_stderr_contains "maxConvergenceIterations" "S1 stderr names maxConvergenceIterations"
assert_stderr_contains "bubbles.workflow" "S1 stderr names offending agent"
assert_stderr_contains "blocked" "S1 stderr documents 'blocked' remediation"

# =============================================================================
# Scenario S2: iterationCount = 10 -> exit 0, stdout contains PASS
# =============================================================================

note "Scenario S2: iterationCount=10 at cap=10 should exit 0"

S2_ROOT="$WORKSPACE/s2"
stage_repo_root "$S2_ROOT" 10
write_session_json "$S2_ROOT" '{
  "convergenceLoops": [
    {
      "specDir": "specs/900-convergence-fixture",
      "agent": "bubbles.workflow",
      "iterationCount": 10,
      "lastIterationAt": "2026-06-01T10:00:00Z",
      "cappedAt": null
    }
  ]
}'

run_guard "$S2_ROOT" "specs/900-convergence-fixture" >/dev/null

assert_exit 0 "S2 exit code (cap exactly hit)"
assert_stdout_contains "PASS Gate G082" "S2 PASS marker on stdout"
assert_stdout_contains "observed=10" "S2 reports observed=10"

# =============================================================================
# Scenario S3: malformed session.json -> exit 2 + diagnostic
# =============================================================================

note "Scenario S3: malformed session.json should exit 2"

S3_ROOT="$WORKSPACE/s3"
stage_repo_root "$S3_ROOT" 10
# Intentionally malformed JSON.
write_session_json "$S3_ROOT" '{"convergenceLoops": ['

run_guard "$S3_ROOT" "specs/900-convergence-fixture" >/dev/null

assert_exit 2 "S3 exit code (malformed JSON)"
assert_stderr_contains "convergence-cap-guard" "S3 stderr has diagnostic prefix"
assert_stderr_contains "not valid JSON" "S3 stderr names malformed-JSON condition"

# =============================================================================
# Bonus scenario S4: spec filter — entry for a DIFFERENT spec MUST NOT
# fail the current spec.
# =============================================================================

note "Scenario S4: convergenceLoops[] entry for a different spec should NOT trip the guard"

S4_ROOT="$WORKSPACE/s4"
stage_repo_root "$S4_ROOT" 10
write_session_json "$S4_ROOT" '{
  "convergenceLoops": [
    {
      "specDir": "specs/999-other-spec",
      "agent": "bubbles.workflow",
      "iterationCount": 99,
      "lastIterationAt": "2026-06-01T10:00:00Z",
      "cappedAt": null
    }
  ]
}'

run_guard "$S4_ROOT" "specs/900-convergence-fixture" >/dev/null

assert_exit 0 "S4 exit code (other-spec entry isolated)"
assert_stdout_contains "observed=0" "S4 ignores entries for non-matching specDir"

# =============================================================================
# Final verdict
# =============================================================================

echo ""
echo "============================================================"
echo "  CONVERGENCE-CAP-GUARD SELFTEST VERDICT"
echo "============================================================"
printf 'Passed assertions: %d\n' "$PASS_COUNT"
printf 'Failed assertions: %d\n' "$FAIL_COUNT"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
  echo ""
  echo "FAILED scenarios:"
  for s in "${FAILED_SCENARIOS[@]}"; do
    echo "  - $s"
  done
  exit 1
fi

echo ""
echo "🟢 convergence-cap-guard-selftest: ALL SCENARIOS PASS"
exit 0
