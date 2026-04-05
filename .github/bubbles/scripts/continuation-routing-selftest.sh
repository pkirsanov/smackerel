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

echo "Running continuation-routing regression selftest..."
echo "Scenario: stochastic-quality-sweep finishes a round, user says 'fix all found', workflow must preserve workflow-owned continuation."

check_pattern "$ROOT_DIR/../agents/bubbles.workflow.agent.md" 'fix all found|fix everything found|address rest|fix the rest' "Workflow agent recognizes continuation-shaped follow-up vocabulary"
check_pattern "$ROOT_DIR/../agents/bubbles.workflow.agent.md" 'attempt active-workflow resume first' "Workflow agent attempts active workflow resume before iterate fallback"
check_pattern "$ROOT_DIR/../agents/bubbles.workflow.agent.md" 'preserve that mode\. Do NOT silently collapse it to `delivery-lockdown`' "Workflow agent preserves stochastic sweep mode during resume"
check_pattern "$ROOT_DIR/../agents/bubbles.workflow.agent.md" 'preferredWorkflowMode: stochastic-quality-sweep' "Workflow agent emits workflow-owned continuation packets for stochastic sweeps"
check_pattern "$ROOT_DIR/../agents/bubbles.super.agent.md" 'fix all found from the last sweep' "Super agent documents the stochastic sweep continuation example"
check_pattern "$ROOT_DIR/../agents/bubbles.super.agent.md" 'Preserve `stochastic-quality-sweep`, `iterate`, and `delivery-lockdown`' "Super agent continuation guard preserves active workflow modes"
check_pattern "$ROOT_DIR/../docs/recipes/resume-work.md" 'tries to resume the active workflow context first' "Resume recipe documents active-workflow resume precedence"
check_pattern "$ROOT_DIR/../docs/recipes/quality-sweep.md" 'fix all found|address the rest' "Quality sweep recipe documents workflow-owned continuation language"
check_pattern "$ROOT_DIR/../docs/guides/WORKFLOW_MODES.md" 'resumes? the active workflow when continuation context is available' "Workflow modes guide documents continuation resume precedence"

if [[ "$failures" -gt 0 ]]; then
  echo "continuation-routing selftest failed with $failures issue(s)."
  exit 1
fi

echo "continuation-routing selftest passed."