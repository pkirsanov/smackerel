#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  ROOT_DIR="$(cd "$SCRIPT_DIR/../../.." && pwd)"
  AGENTS_DIR="$ROOT_DIR/.github/agents"
  WORKFLOWS_FILE="$ROOT_DIR/.github/bubbles/workflows.yaml"
else
  ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
  AGENTS_DIR="$ROOT_DIR/agents"
  WORKFLOWS_FILE="$ROOT_DIR/bubbles/workflows.yaml"
fi
# v6.1 (S2 true split): mode-internal registries (triggerWorkflowModes, per-mode
# finding routing) live in bubbles/workflows/modes.yaml. Check those patterns
# there; fall back to workflows.yaml for pre-split repos with an inline modes:
# block.
MODES_FILE="$(dirname "$WORKFLOWS_FILE")/workflows/modes.yaml"
[[ -f "$MODES_FILE" ]] || MODES_FILE="$WORKFLOWS_FILE"

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

check_pattern "$AGENTS_DIR/bubbles_shared/critical-requirements.md" '^14\. \*\*No Selective Remediation Of Discovered Findings\*\*$' "Critical requirements forbid selective remediation of discovered findings"
check_pattern "$AGENTS_DIR/bubbles_shared/critical-requirements.md" 'Fixing the easy subset while narrating the rest as .*later.*incomplete work' "Critical requirements reject easy-subset remediation language"
check_pattern "$AGENTS_DIR/bubbles_shared/completion-governance.md" '^## Finding-Set Closure Is Mandatory$' "Completion governance defines mandatory finding-set closure"
check_pattern "$AGENTS_DIR/bubbles_shared/completion-governance.md" 'timing attack is fixable now.*JWT migration is a larger change' "Completion governance documents the invalid timing-attack/JWT cherry-pick pattern"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'You MUST account for every finding individually' "Workflow agent instructs implement to account for every finding individually"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'Require one-to-one accounting against the finding list' "Workflow agent verifies one-to-one finding accounting"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'Every finding was accounted for' "Workflow agent post-fix-cycle verification checks full finding accounting"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'Include the full finding ledger in the implement prompt and require one-to-one closure accounting' "Sequential findings handling carries the full finding ledger forward"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'Full finding-owned planning workflow:' "Workflow agent defines the full finding-owned planning workflow"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'bubbles\.analyst.*bubbles\.ux.*bubbles\.design.*bubbles\.plan' "Workflow agent routes findings through analyst ux design plan"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'Full finding-owned delivery workflow:' "Workflow agent defines the full finding-owned delivery workflow"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'bubbles\.implement.*bubbles\.test.*bubbles\.validate.*bubbles\.audit.*bubbles\.docs' "Workflow agent routes findings through implement test validate audit docs"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'This applies to `chaos`, `test`, `simplify`, `stabilize`, `devops`, `security`, `validate`, `regression`, `harden`, `gaps`' "Workflow agent applies finding-owned closure to all trigger-style specialists"
check_pattern "$MODES_FILE" 'triggerWorkflowModes:' "Workflow registry defines trigger-owned workflow mappings"
check_pattern "$MODES_FILE" 'test: test-to-doc' "Workflow registry maps test findings to a trigger-owned mapped workflow"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'MUST NOT execute the trigger phase as a standalone probe or build an ad hoc trigger-specific fix cycle' "Stochastic runner is forbidden from running trigger-only rounds when a mapped mode exists"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'executionModel: direct-authorized-runner' "Stochastic sweep uses direct authorized execution"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'owns the full chain from its trigger through the finding-owned planning workflow' "Trigger-owned mapped workflow owns the finding-owned planning chain"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'MUST NOT rerun a bespoke docs/finalize tail per spec' "Stochastic runner does not duplicate docs/finalize after mapped mode completion"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'MUST NOT end in summary-only output while any touched spec or any round remains' "Stochastic sweep forbids summary-only completion when non-terminal work remains"
check_pattern "$AGENTS_DIR/bubbles_shared/workflow-fix-cycle-protocol.md" 'MUST execute the mapped workflow contract instead of pre-running the trigger or hand-building a bespoke fix cycle' "Shared trigger protocol forbids bespoke runner-side fix cycles"
check_pattern "$AGENTS_DIR/bubbles_shared/workflow-fix-cycle-protocol.md" 'Any specialist or mapped mode that discovers a legitimate bug, regression, design gap, operational gap, or improvement MUST complete a finding-owned closure workflow' "Shared trigger protocol requires finding-owned closure workflow"
check_pattern "$AGENTS_DIR/bubbles_shared/workflow-fix-cycle-protocol.md" 'Wait for every mapped phase owner to return a terminal `## RESULT-ENVELOPE`' "Shared trigger protocol requires terminal mapped-mode phase results"
check_pattern "$AGENTS_DIR/bubbles_shared/workflow-fix-cycle-protocol.md" 'Mapped workflow modes own docs/finalize/certification for touched specs' "Shared trigger protocol assigns closeout ownership to mapped modes"
check_pattern "$AGENTS_DIR/bubbles_shared/workflow-fix-cycle-protocol.md" 'executionModel=direct-authorized-runner' "Shared trigger protocol records direct authorized execution"
check_pattern "$AGENTS_DIR/bubbles.implement.agent.md" 'account for EVERY routed finding individually' "Implement agent forbids cherry-picking routed findings"
check_pattern "$AGENTS_DIR/bubbles.implement.agent.md" '`addressedFindings`' "Implement agent requires addressedFindings in the result envelope"
check_pattern "$AGENTS_DIR/bubbles.implement.agent.md" '`unresolvedFindings`' "Implement agent requires unresolvedFindings in the result envelope"
check_pattern "$AGENTS_DIR/bubbles.implement.agent.md" 'completed_owned.*unresolvedFindings.*empty' "Implement agent blocks completed_owned when unresolved findings remain"

# Stochastic sweep round loop must be synchronous and dispatch-then-wait
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'SYNCHRONOUS ROUND LOOP.*Each round MUST execute the mapped workflow contract.*WAIT' "Workflow agent enforces synchronous execute-and-wait per round"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'Batching round selections without executing mapped modes is FORBIDDEN' "Workflow agent forbids batch-then-summarize round execution"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'No report-only completion.*Producing a table of findings without executing mapped workflow modes' "Workflow agent forbids report-only sweep completion"
check_pattern "$AGENTS_DIR/bubbles_shared/workflow-execution-loops.md" 'CRITICAL: Each round MUST complete.*including full mapped-mode remediation.*before the next round starts' "Execution loops enforce synchronous round completion"
check_pattern "$AGENTS_DIR/bubbles_shared/workflow-execution-loops.md" 'No batch-then-summarize.*MUST NOT generate all N round selections first' "Execution loops explicitly prohibit batch-then-summarize pattern"
check_pattern "$AGENTS_DIR/bubbles_shared/workflow-execution-loops.md" 'No report-only completion.*Producing a table of findings without executing mapped workflow modes' "Execution loops forbid report-only sweep output"
check_pattern "$AGENTS_DIR/bubbles_shared/workflow-execution-loops.md" 'Execute the mapped workflow mode' "Execution loops require mapped mode execution"
check_pattern "$AGENTS_DIR/bubbles_shared/workflow-execution-loops.md" 'executionModel=direct-authorized-runner' "Execution loops define direct authorized execution"
check_pattern "$AGENTS_DIR/bubbles_shared/workflow-execution-loops.md" 'WAIT for the mapped mode to return a terminal.*RESULT-ENVELOPE.*Do NOT proceed to the next round' "Execution loops require waiting for mapped mode before next round"
check_pattern "$AGENTS_DIR/bubbles_shared/workflow-fix-cycle-protocol.md" 'execute -> wait -> record for one round before starting the next' "Fix-cycle protocol enforces per-round synchronous completion in round-based loops"

if [[ "$failures" -gt 0 ]]; then
  echo "finding-closure selftest failed with $failures issue(s)."
  exit 1
fi

echo "finding-closure selftest passed."