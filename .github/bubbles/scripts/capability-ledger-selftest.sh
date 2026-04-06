#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

failures=0

pass() {
  echo "PASS: $1"
}

fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

check_pattern() {
  local file_path="$1"
  local pattern="$2"
  local label="$3"

  if grep -Eq "$pattern" "$file_path"; then
    pass "$label"
  else
    fail "$label"
  fi
}

echo "Running capability-ledger selftest..."
echo "Scenario: ledger-backed competitive docs stay aligned with the source-of-truth registry."

bash "$SCRIPT_DIR/generate-capability-ledger-docs.sh" --check >/dev/null
pass "Capability ledger generated surfaces are current"

check_pattern "$ROOT_DIR/bubbles/capability-ledger.yaml" '^  workflow-orchestration:$' "Ledger defines workflow orchestration capability"
check_pattern "$ROOT_DIR/bubbles/capability-ledger.yaml" '^  supported-interop-apply:$' "Ledger defines supported interop apply capability"
check_pattern "$ROOT_DIR/bubbles/capability-ledger.yaml" '^  session-aware-runtime-coordination:$' "Ledger tracks runtime coordination proposal"
check_pattern "$ROOT_DIR/docs/generated/competitive-capabilities.md" '^State summary: 4 shipped, 1 partial, 2 proposed\.$' "Generated capability guide exposes stable state summary"
check_pattern "$ROOT_DIR/docs/generated/competitive-capabilities.md" '\| Workflow orchestration \| shipped \|' "Generated capability guide includes shipped workflow orchestration row"
check_pattern "$ROOT_DIR/docs/generated/competitive-capabilities.md" '\| Supported interop apply \| shipped \|' "Generated capability guide includes shipped supported interop apply row"
check_pattern "$ROOT_DIR/docs/generated/competitive-capabilities.md" '\| Session-aware runtime coordination \| proposed \|' "Generated capability guide includes proposed runtime coordination row"
check_pattern "$ROOT_DIR/docs/generated/issue-status.md" '^Tracked gaps: 2 issue-backed capabilities\.$' "Generated issue status guide counts tracked gaps from the ledger"
check_pattern "$ROOT_DIR/docs/generated/interop-migration-matrix.md" '\| Claude Code \| markdown \|' "Generated migration matrix is refreshed from the interop registry"

if [[ "$failures" -gt 0 ]]; then
  echo "capability-ledger selftest failed with $failures issue(s)."
  exit 1
fi

echo "capability-ledger selftest passed."