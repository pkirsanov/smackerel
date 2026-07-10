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

check_consumers_exist() {
  local capability="$1" label="$2"
  local consumers=()
  local consumer

  if ! command -v yq >/dev/null 2>&1; then
    fail "$label consumers cannot be checked because yq is missing"
    return 0
  fi

  mapfile -t consumers < <(yq -r '.capabilities["'"$capability"'"].consumers[]?' "$ROOT_DIR/bubbles/capability-ledger.yaml" 2>/dev/null || true)
  if [[ "${#consumers[@]}" -eq 0 ]]; then
    fail "$label declares at least one consumer"
    return 0
  fi
  pass "$label declares ${#consumers[@]} consumer path(s)"

  for consumer in "${consumers[@]}"; do
    if [[ -e "$ROOT_DIR/$consumer" ]]; then
      pass "$label consumer path exists: $consumer"
    else
      fail "$label consumer path is missing: $consumer"
    fi
  done
}

echo "Running capability-ledger selftest..."
echo "Scenario: ledger-backed competitive docs stay aligned with the source-of-truth registry."

bash "$SCRIPT_DIR/generate-capability-ledger-docs.sh" --check >/dev/null
pass "Capability ledger generated surfaces are current"

check_pattern "$ROOT_DIR/bubbles/capability-ledger.yaml" '^  workflow-orchestration:$' "Ledger defines workflow orchestration capability"
check_pattern "$ROOT_DIR/bubbles/capability-ledger.yaml" '^  supported-interop-apply:$' "Ledger defines supported interop apply capability"
check_pattern "$ROOT_DIR/bubbles/capability-ledger.yaml" '^  session-aware-runtime-coordination:$' "Ledger defines runtime coordination capability"
check_pattern "$ROOT_DIR/bubbles/capability-ledger.yaml" '^  orchestrator-context-compaction:$' "Ledger defines orchestrator context compaction capability"
check_pattern "$ROOT_DIR/bubbles/capability-ledger.yaml" '^  per-turn-state-snapshot:$' "Ledger defines per-turn state snapshot capability"
check_pattern "$ROOT_DIR/bubbles/capability-ledger.yaml" '^  linter-on-edit-gate:$' "Ledger defines linter-on-edit gate capability"
check_pattern "$ROOT_DIR/docs/generated/competitive-capabilities.md" '^State summary: 22 shipped, 1 partial, 0 proposed\.$' "Generated capability guide exposes stable state summary"
check_pattern "$ROOT_DIR/bubbles/capability-ledger.yaml" '^  workflow-runner-authorization:$' "Ledger defines workflow runner authorization capability"
check_pattern "$ROOT_DIR/docs/generated/competitive-capabilities.md" '\| Workflow orchestration \| shipped \|' "Generated capability guide includes shipped workflow orchestration row"
check_pattern "$ROOT_DIR/docs/generated/competitive-capabilities.md" '\| Workflow runner authorization \| shipped \|' "Generated capability guide includes workflow runner authorization row"
check_pattern "$ROOT_DIR/docs/generated/competitive-capabilities.md" '\| Supported interop apply \| shipped \|' "Generated capability guide includes shipped supported interop apply row"
check_pattern "$ROOT_DIR/docs/generated/competitive-capabilities.md" '\| Session-aware runtime coordination \| shipped \|' "Generated capability guide includes shipped runtime coordination row"
check_pattern "$ROOT_DIR/docs/generated/competitive-capabilities.md" '\| Orchestrator context compaction \| shipped \|' "Generated capability guide includes shipped orchestrator context compaction row"
check_pattern "$ROOT_DIR/docs/generated/competitive-capabilities.md" '\| Per-turn state snapshot \| shipped \|' "Generated capability guide includes shipped per-turn state snapshot row"
check_pattern "$ROOT_DIR/docs/generated/competitive-capabilities.md" '\| Linter-on-edit gate \| shipped \|' "Generated capability guide includes shipped linter-on-edit gate row"
check_pattern "$ROOT_DIR/docs/generated/issue-status.md" '^Tracked gaps: 2 issue-backed capabilities\.$' "Generated issue status guide counts tracked gaps from the ledger"
check_pattern "$ROOT_DIR/docs/generated/interop-migration-matrix.md" '\| Claude Code \| markdown \|' "Generated migration matrix is refreshed from the interop registry"

check_consumers_exist "observability-adapter-contract" "Observability adapter contract"
check_consumers_exist "observability-posture-and-slo-gates" "Observability posture/SLO gates"

if [[ "$failures" -gt 0 ]]; then
  echo "capability-ledger selftest failed with $failures issue(s)."
  exit 1
fi

echo "capability-ledger selftest passed."