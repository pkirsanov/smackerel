#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
SCAN_SCRIPT="$SCRIPT_DIR/implementation-reality-scan.sh"
FIXTURE_ROOT="$REPO_DIR/specs/001-competitive-framework-parity/bugs/BUG-001-shell-heavy-reality-scan-discovery-gap/fixtures"

failures=0

pass() {
  echo "PASS: $1"
}

fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

run_expect_success() {
  local feature_dir="$1"
  local label="$2"
  local output=""

  if output="$(timeout 180 bash "$SCAN_SCRIPT" "$feature_dir" --verbose 2>&1)"; then
    echo "$output"
    pass "$label"
  else
    echo "$output"
    fail "$label"
  fi
}

run_expect_zero_files_failure() {
  local feature_dir="$1"
  local label="$2"
  local output=""
  local status=0

  if output="$(timeout 180 bash "$SCAN_SCRIPT" "$feature_dir" --verbose 2>&1)"; then
    echo "$output"
    fail "$label"
    return
  else
    status=$?
    echo "$output"
  fi

  if [[ "$status" -eq 1 ]] && grep -Fq 'ZERO_FILES_RESOLVED' <<< "$output"; then
    pass "$label"
  else
    fail "$label"
  fi
}

echo "Running implementation-reality-scan discovery selftest..."
echo "Scenario: shell-heavy fixtures resolve honest implementation inventory."
run_expect_success "$FIXTURE_ROOT/shell-heavy-feature" "Shell-heavy fixture resolves .sh/.yaml/.yml/.json/docs-backed inventory"

echo "Scenario: missing inventories still fail with ZERO_FILES_RESOLVED."
run_expect_zero_files_failure "$FIXTURE_ROOT/missing-inventory-feature" "Missing-inventory fixture fails honestly without shim files"

if [[ "$failures" -gt 0 ]]; then
  echo "implementation-reality-scan selftest failed with $failures issue(s)."
  exit 1
fi

echo "implementation-reality-scan selftest passed."