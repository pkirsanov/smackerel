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

echo "Running super-surface awareness selftest..."

SUPER_AGENT="$ROOT_DIR/../agents/bubbles.super.agent.md"
SUPER_PROMPT="$ROOT_DIR/../prompts/bubbles.super.prompt.md"
AGENT_MANUAL="$ROOT_DIR/../docs/guides/AGENT_MANUAL.md"
SUPER_RECIPE="$ROOT_DIR/../docs/recipes/ask-the-super-first.md"

check_pattern "$SUPER_AGENT" 'Source repo: `ls agents/bubbles\.\*\.agent\.md`; downstream repo: `ls \.github/agents/bubbles\.\*\.agent\.md`' "Super agent discovers source and downstream agent inventories"
check_pattern "$SUPER_AGENT" 'source repo `bubbles/workflows\.yaml`, downstream repo `\.github/bubbles/workflows\.yaml`' "Super agent discovers workflow registry by repo posture"
check_pattern "$SUPER_AGENT" 'Source repo: `ls skills/\*/SKILL\.md`; downstream repo: `ls \.github/skills/\*/SKILL\.md`' "Super agent discovers source and downstream skills"
check_pattern "$SUPER_AGENT" 'Source repo: `ls instructions/\*\.md`; downstream repo: `ls \.github/instructions/\*\.instructions\.md`' "Super agent discovers source and downstream instructions"
check_pattern "$SUPER_AGENT" 'docs/CATALOG\.md' "Super agent uses the recipe catalog as a feature map"
check_pattern "$SUPER_AGENT" 'framework-events|run-state|repo-readiness|action-risk-registry\.yaml' "Super agent knows the new control-plane command surfaces"
check_pattern "$SUPER_AGENT" 'Feature Coverage Guard' "Super agent documents the broad capability coverage guard"
check_pattern "$SUPER_AGENT" 'Source framework repo \| `bash bubbles/scripts/cli\.sh \.\.\.`' "Super agent documents source-repo CLI path resolution"
check_pattern "$SUPER_AGENT" 'Downstream installed repo \| `bash \.github/bubbles/scripts/cli\.sh \.\.\.`' "Super agent documents downstream CLI path resolution"
check_pattern "$SUPER_PROMPT" 'framework validation, release hygiene, run-state and event diagnostics, repo-readiness' "Super prompt advertises expanded framework ops scope"
check_pattern "$SUPER_PROMPT" 'agents, workflow modes, recipes, skills, instructions, CLI commands, run-state, framework events, and risk classes' "Super prompt requires live-surface discovery"
check_optional_pattern "$AGENT_MANUAL" 'recipe, skill, instruction, risk, and runtime surfaces' "Agent manual documents super surface discovery breadth"
check_optional_pattern "$SUPER_RECIPE" 'source framework repo: `bash bubbles/scripts/cli\.sh \.\.\.`' "Super recipe documents source-repo CLI resolution"
check_optional_pattern "$SUPER_RECIPE" 'downstream installed repo: `bash \.github/bubbles/scripts/cli\.sh \.\.\.`' "Super recipe documents downstream CLI resolution"

if [[ "$failures" -gt 0 ]]; then
  echo "super-surface selftest failed with $failures issue(s)."
  exit 1
fi

echo "super-surface selftest passed."