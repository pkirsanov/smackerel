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

echo "Running workflow-delegation selftest..."
echo "Scenario: workflow orchestration should delegate vague intent routing to super and work picking to iterate instead of duplicating those responsibilities."

check_pattern "$AGENTS_DIR/bubbles_shared/workflow-delegation-core.md" '`bubbles\.super` is the ONLY natural-language dispatcher' "Shared delegation core assigns natural-language routing to super"
check_pattern "$AGENTS_DIR/bubbles_shared/workflow-delegation-core.md" '`bubbles\.iterate` is the ONLY highest-priority work picker' "Shared delegation core assigns work selection to iterate"
check_pattern "$AGENTS_DIR/bubbles_shared/workflow-delegation-core.md" 'MUST NOT recreate a local intent-to-mode keyword table' "Shared delegation core forbids duplicate workflow intent tables"
check_pattern "$AGENTS_DIR/bubbles_shared/workflow-delegation-core.md" 'MUST NOT maintain its own work-priority heuristic' "Shared delegation core forbids duplicate workflow work pickers"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" '`bubbles\.super` is the ONLY natural-language dispatcher' "Workflow agent delegates vague routing to super"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" '`bubbles\.iterate` is the ONLY highest-priority work picker' "Workflow agent delegates work discovery to iterate"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" '^tools: \[.*agent.*\]' "Workflow agent frontmatter exposes subagent tool"
check_pattern "$AGENTS_DIR/bubbles.iterate.agent.md" '^tools: \[.*agent.*\]' "Iterate agent frontmatter exposes subagent tool"
check_pattern "$AGENTS_DIR/bubbles.goal.agent.md" '^tools: \[.*agent.*\]' "Goal agent frontmatter exposes subagent tool"
check_pattern "$AGENTS_DIR/bubbles.sprint.agent.md" '^tools: \[.*agent.*\]' "Sprint agent frontmatter exposes subagent tool"
check_pattern "$AGENTS_DIR/bubbles.bug.agent.md" '^tools: \[.*agent.*\]' "Bug agent frontmatter exposes subagent tool"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'Outcome-first mode dispatch' "Workflow agent has outcome-first mode dispatch rule"
check_pattern "$AGENTS_DIR/bubbles.goal.agent.md" 'Outcome-First Dispatch Contract' "Goal agent has outcome-first dispatch contract"
check_pattern "$AGENTS_DIR/bubbles.sprint.agent.md" 'Outcome-First Dispatch Contract' "Sprint agent has outcome-first dispatch contract"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'MUST NOT maintain its own intent-to-mode mapping table' "Workflow agent forbids duplicate intent mapping"
check_pattern "$AGENTS_DIR/bubbles.workflow.agent.md" 'MUST NOT maintain its own work-priority heuristic' "Workflow agent forbids duplicate work-priority logic"
check_pattern "$AGENTS_DIR/bubbles.super.agent.md" 'owns natural-language translation into workflow parameters' "Super agent claims natural-language translation ownership"
check_pattern "$AGENTS_DIR/bubbles.iterate.agent.md" 'owns highest-priority work selection and `WORK-ENVELOPE` output' "Iterate agent claims work-envelope ownership"
check_pattern "$ROOT_DIR/docs/guides/WORKFLOW_MODES.md" 'workflow routes plain-English requests through `super`' "Workflow modes guide routes plain-English requests through super"
check_pattern "$ROOT_DIR/docs/guides/WORKFLOW_MODES.md" '`bubbles\.iterate` owns generic work-picking' "Workflow modes guide documents iterate ownership"
check_pattern "$ROOT_DIR/bubbles/agent-capabilities.yaml" '^  bubbles\.goal:' "Agent capabilities manifest declares goal orchestrator"
check_pattern "$ROOT_DIR/bubbles/agent-capabilities.yaml" '^  bubbles\.sprint:' "Agent capabilities manifest declares sprint orchestrator"
check_pattern "$ROOT_DIR/bubbles/workflows.yaml" 'modeOrAgentFit:' "Workflow policy defines better-fit mode/agent escalation"
check_pattern "$ROOT_DIR/bubbles/workflows.yaml" 'missingAgentTool:' "Workflow policy defines missing agent-tool blocking outcome"

if [[ "$failures" -gt 0 ]]; then
  echo "workflow-delegation selftest failed with $failures issue(s)."
  exit 1
fi

echo "workflow-delegation selftest passed."