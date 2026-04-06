#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  ROOT_DIR="$(cd "$SCRIPT_DIR/../../.." && pwd)"
else
  ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

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

echo "Running finding-closure selftest..."
echo "Scenario: finding-driven workflow rounds must reject cherry-picking easy fixes while narrating harder findings away."

check_pattern "$ROOT_DIR/.github/agents/bubbles_shared/critical-requirements.md" '^14\. \*\*No Selective Remediation Of Discovered Findings\*\*$' "Critical requirements forbid selective remediation of discovered findings"
check_pattern "$ROOT_DIR/.github/agents/bubbles_shared/critical-requirements.md" 'Fixing the easy subset while narrating the rest as .*later.*incomplete work' "Critical requirements reject easy-subset remediation language"
check_pattern "$ROOT_DIR/.github/agents/bubbles_shared/completion-governance.md" '^## Finding-Set Closure Is Mandatory$' "Completion governance defines mandatory finding-set closure"
check_pattern "$ROOT_DIR/.github/agents/bubbles_shared/completion-governance.md" 'timing attack is fixable now.*JWT migration is a larger change' "Completion governance documents the invalid timing-attack/JWT cherry-pick pattern"
check_pattern "$ROOT_DIR/.github/agents/bubbles.workflow.agent.md" 'You MUST account for every finding individually' "Workflow agent instructs implement to account for every finding individually"
check_pattern "$ROOT_DIR/.github/agents/bubbles.workflow.agent.md" 'Require one-to-one accounting against the finding list' "Workflow agent verifies one-to-one finding accounting"
check_pattern "$ROOT_DIR/.github/agents/bubbles.workflow.agent.md" 'Every finding was accounted for' "Workflow agent post-fix-cycle verification checks full finding accounting"
check_pattern "$ROOT_DIR/.github/agents/bubbles.workflow.agent.md" 'Include the full finding ledger in the implement prompt and require one-to-one closure accounting' "Sequential findings handling carries the full finding ledger forward"
check_pattern "$ROOT_DIR/.github/agents/bubbles.implement.agent.md" 'account for EVERY routed finding individually' "Implement agent forbids cherry-picking routed findings"
check_pattern "$ROOT_DIR/.github/agents/bubbles.implement.agent.md" '`addressedFindings`' "Implement agent requires addressedFindings in the result envelope"
check_pattern "$ROOT_DIR/.github/agents/bubbles.implement.agent.md" '`unresolvedFindings`' "Implement agent requires unresolvedFindings in the result envelope"
check_pattern "$ROOT_DIR/.github/agents/bubbles.implement.agent.md" 'completed_owned.*unresolvedFindings.*empty' "Implement agent blocks completed_owned when unresolved findings remain"

if [[ "$failures" -gt 0 ]]; then
  echo "finding-closure selftest failed with $failures issue(s)."
  exit 1
fi

echo "finding-closure selftest passed."