#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  ROOT_DIR="$(cd "$SCRIPT_DIR/../../.." && pwd)"
  AGENTS_DIR="$ROOT_DIR/.github/agents"
else
  ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
  AGENTS_DIR="$ROOT_DIR/agents"
fi

# Asset roots: docs/, bubbles/ live at repo root in the framework source tree
# and under .github/ in downstream installs. Resolve once.
if [[ -d "$ROOT_DIR/docs" && -f "$ROOT_DIR/docs/guides/WORKFLOW_MODES.md" ]]; then
  DOCS_DIR="$ROOT_DIR/docs"
else
  DOCS_DIR="$ROOT_DIR/.github/docs"
fi
if [[ -f "$ROOT_DIR/bubbles/workflows.yaml" ]]; then
  BUBBLES_DIR="$ROOT_DIR/bubbles"
else
  BUBBLES_DIR="$ROOT_DIR/.github/bubbles"
fi
# v6.1 (S2 true split): mode definitions live in bubbles/workflows/modes.yaml,
# not inline in workflows.yaml. Mode-internal policy patterns are checked there;
# fall back to workflows.yaml for pre-split repos with an inline modes: block.
MODES_FILE="$BUBBLES_DIR/workflows/modes.yaml"
[[ -f "$MODES_FILE" ]] || MODES_FILE="$BUBBLES_DIR/workflows.yaml"

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

check_no_pattern() {
  local file_path="$1"
  local pattern="$2"
  local label="$3"

  if grep -Eq "$pattern" "$file_path"; then
    fail "$label"
  else
    pass "$label"
  fi
}

echo "Running workflow-delegation selftest..."
echo "Scenario: workflow orchestration should delegate vague intent routing to super and work picking to iterate instead of duplicating those responsibilities."

check_pattern "$AGENTS_DIR/bubbles_shared/workflow-delegation-core.md" '`bubbles\.super` is the ONLY natural-language dispatcher' "Shared delegation core assigns natural-language routing to super"
check_pattern "$AGENTS_DIR/bubbles_shared/workflow-delegation-core.md" '`bubbles\.iterate` is the ONLY highest-priority work picker' "Shared delegation core assigns work selection to iterate"
check_pattern "$AGENTS_DIR/bubbles_shared/workflow-delegation-core.md" 'MUST NOT recreate a local intent-to-mode keyword table' "Shared delegation core forbids duplicate workflow intent tables"
check_pattern "$AGENTS_DIR/bubbles_shared/workflow-delegation-core.md" 'MUST NOT maintain its own work-priority heuristic' "Shared delegation core forbids duplicate workflow work pickers"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" '`bubbles\.super` is the ONLY natural-language dispatcher' "Workflow agent delegates vague routing to super"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'returns `route_required` to that registered meta-mode owner' "Workflow agent routes work discovery to iterate without nesting it"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" '^tools: \[.*agent.*\]' "Workflow agent frontmatter exposes subagent tool"
check_pattern "$AGENTS_DIR/bubbles.iterate.agent.md" '^tools: \[.*agent.*\]' "Iterate agent frontmatter exposes subagent tool"
check_pattern "$AGENTS_DIR/bubbles.goal.agent.md" '^tools: \[.*agent.*\]' "Goal agent frontmatter exposes subagent tool"
check_pattern "$AGENTS_DIR/bubbles.sprint.agent.md" '^tools: \[.*agent.*\]' "Sprint agent frontmatter exposes subagent tool"
check_pattern "$AGENTS_DIR/bubbles.bug.agent.md" '^tools: \[.*agent.*\]' "Bug agent frontmatter exposes subagent tool"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'Mode fidelity' "Workflow agent enforces its single-mode boundary"
check_pattern "$AGENTS_DIR/bubbles.goal.agent.md" 'Outcome-First Dispatch Contract' "Goal agent has outcome-first dispatch contract"
check_pattern "$AGENTS_DIR/bubbles.sprint.agent.md" 'Outcome-First Dispatch Contract' "Sprint agent has outcome-first dispatch contract"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'MUST NOT maintain its own intent-to-mode mapping table' "Workflow agent forbids duplicate intent mapping"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'MUST NOT maintain its own work-priority heuristic' "Workflow agent forbids duplicate work-priority logic"
check_pattern "$AGENTS_DIR/bubbles.super.agent.md" 'owns natural-language translation into workflow parameters' "Super agent claims natural-language translation ownership"
check_pattern "$AGENTS_DIR/bubbles.iterate.agent.md" 'returns a `WORK-ENVELOPE` and does not execute a workflow' "Iterate agent preserves picker-only behavior"
check_pattern "$DOCS_DIR/guides/WORKFLOW_MODES.md" 'workflow routes plain-English requests through `super`' "Workflow modes guide routes plain-English requests through super"
check_pattern "$DOCS_DIR/guides/WORKFLOW_MODES.md" 'request resolves to `iterate`.*workflow returns the registered top-level owner' "Workflow modes guide documents iterate meta-mode routing"
check_pattern "$BUBBLES_DIR/agent-capabilities.yaml" '^  bubbles\.goal:' "Agent capabilities manifest declares goal orchestrator"
check_pattern "$BUBBLES_DIR/agent-capabilities.yaml" '^  bubbles\.sprint:' "Agent capabilities manifest declares sprint orchestrator"
check_pattern "$BUBBLES_DIR/workflows.yaml" 'modeOrAgentFit:' "Workflow policy defines better-fit mode/agent escalation"
check_pattern "$BUBBLES_DIR/workflows.yaml" 'missingAgentTool:' "Workflow policy defines missing agent-tool blocking outcome"
check_pattern "$MODES_FILE" 'allowDirectMappedWorkflowExecution: true' "Workflow policy enables direct mapped workflow execution"

# Direct authorized workflow execution. VS Code subagents cannot dispatch another
# subagent, so top-level orchestrators must execute granted modes themselves
# instead of spawning another workflow-running orchestrator.
check_pattern "$BUBBLES_DIR/workflows.yaml" '^workflowExecutionPolicy:' "Workflow policy declares authorized mode runners"
check_pattern "$BUBBLES_DIR/workflows.yaml" 'defaultExecutionModel: direct-authorized-runner' "Workflow policy defaults to direct authorized runners"
check_pattern "$BUBBLES_DIR/agent-capabilities.yaml" '^workflowModeGrants:' "Capability registry declares per-agent workflow grants"
check_pattern "$AGENTS_DIR/bubbles_shared/workflow-delegation-core.md" 'executionModel: direct-authorized-runner' "Shared delegation core defines direct workflow execution"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'exactly one resolved workflow mode' "Workflow agent is a single-mode runner"
check_pattern "$AGENTS_DIR/bubbles.goal.agent.md" 'Universal goal endpoint' "Goal agent is the universal outcome endpoint"
check_no_pattern "$AGENTS_DIR/bubbles.goal.agent.md" 'preferred: runSubagent\(bubbles\.workflow\)' "Goal agent must not prefer nested workflow-agent execution"
check_no_pattern "$AGENTS_DIR/bubbles.goal.agent.md" 'preferred — single-call delegation' "Goal agent must execute workflow definitions directly"
check_no_pattern "$AGENTS_DIR/bubbles.sprint.agent.md" 'prefer one runSubagent\(bubbles\.goal\)' "Sprint must not prefer nested goal-agent execution"
check_no_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'invoke `bubbles\.iterate` for generic work discovery' "Workflow must not invoke iterate as an unbounded fallback"
check_pattern "$AGENTS_DIR/bubbles.sprint.agent.md" 'direct-authorized-runner' "Sprint directly executes granted goal workflows"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'direct-authorized-runner' "Workflow agent documents direct authorized execution"
check_pattern "$AGENTS_DIR/bubbles_shared/workflow-delegation-core.md" 'Do not assume a subagent can invoke another subagent' "Delegation core documents one-level runtime compatibility"
check_pattern "$AGENTS_DIR/bubbles.goal.agent.md" 'execute_mode_directly: bugfix-fastlane' "Goal directly executes bug remediation modes"
check_pattern "$AGENTS_DIR/bubbles.sprint.agent.md" 'Never invoke `bubbles.goal`' "Sprint forbids nested goal execution"

if [[ "$failures" -gt 0 ]]; then
  echo "workflow-delegation selftest failed with $failures issue(s)."
  exit 1
fi

echo "workflow-delegation selftest passed."