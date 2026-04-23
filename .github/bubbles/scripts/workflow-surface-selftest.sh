#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

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

check_optional_pattern() {
  local file_path="$1"
  local pattern="$2"
  local label="$3"

  if [[ ! -f "$file_path" ]]; then
    echo "SKIP: $label (missing $(basename "$file_path"))"
    return 0
  fi

  check_pattern "$file_path" "$pattern" "$label"
}

echo "Running workflow command-surface smoke test..."

check_pattern "$ROOT_DIR/workflows.yaml" '^  full-delivery:$' "Workflow registry exposes full-delivery"
check_pattern "$ROOT_DIR/workflows.yaml" '^    specReview:$' "Workflow registry exposes the specReview execution option"
check_pattern "$SCRIPT_DIR/aliases.sh" '\[no-loose-ends\]="full-delivery"' "Sunnyvale alias resolves to full-delivery"
check_pattern "$ROOT_DIR/../agents/bubbles.workflow.agent.md" 'mode: .*full-delivery' "Workflow agent advertises full-delivery mode"
check_pattern "$ROOT_DIR/../agents/bubbles.workflow.agent.md" 'Phase 0\.95: Full-Delivery Convergence Loop' "Workflow agent documents the full-delivery convergence loop"
check_pattern "$ROOT_DIR/../agents/bubbles.super.agent.md" 'full-delivery' "Super agent knows about full-delivery"
check_pattern "$ROOT_DIR/../agents/bubbles.super.agent.md" 'no loose ends|until all green|release-candidate' "Super agent recognizes the lockdown request vocabulary"
check_pattern "$ROOT_DIR/../agents/bubbles.super.agent.md" 'specReview: once-before-implement|stale-spec check|Front-Door Policy' "Super agent exposes the one-shot spec review capability and front-door policy"
check_pattern "$ROOT_DIR/../agents/bubbles.super.agent.md" 'runtime lease|runtime doctor|shared Docker reuse' "Super agent exposes runtime coordination guidance"
check_optional_pattern "$ROOT_DIR/../docs/CHEATSHEET.md" '\| `full-delivery` \| full-send \|' "Cheatsheet exposes the full-delivery alias"
check_optional_pattern "$ROOT_DIR/../docs/CHEATSHEET.md" 'bubbles runtime leases|bubbles runtime doctor|bubbles runtime summary' "Cheatsheet exposes runtime coordination commands"
check_optional_pattern "$ROOT_DIR/../docs/recipes/ask-the-super-first.md" 'full-delivery' "Super recipe demonstrates full-delivery guidance"
check_optional_pattern "$ROOT_DIR/../docs/recipes/ask-the-super-first.md" 'runtime lease conflicts|reuse the validation stack if it is compatible' "Super recipe demonstrates runtime coordination guidance"
check_optional_pattern "$ROOT_DIR/../docs/its-not-rocket-appliances.html" 'wf-name">full-delivery<' "HTML cheat sheet exposes the workflow card"
check_optional_pattern "$ROOT_DIR/../docs/its-not-rocket-appliances.html" 'Lease the lot|Same stack, same lease|Runtime doctor' "HTML cheat sheet exposes runtime TPB vocabulary"

if bash "$SCRIPT_DIR/workflow-registry-consistency.sh" --quiet; then
  pass "Workflow registry consistency check"
else
  fail "Workflow registry consistency check"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "workflow-surface selftest failed with $failures issue(s)."
  exit 1
fi

echo "workflow-surface selftest passed."