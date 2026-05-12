#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
else
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

failures=0

declare -a agnosticity_targets=(
  "CHANGELOG.md"
  "README.md"
  "docs/CHEATSHEET.md"
  "docs/guides/INSTALLATION.md"
  "docs/recipes/framework-ops.md"
  "agents/bubbles.super.agent.md"
  "bubbles/action-risk-registry.yaml"
  "bubbles/scripts/cli.sh"
  "bubbles/scripts/repo-readiness.sh"
  "bubbles/scripts/framework-validate.sh"
  "bubbles/scripts/release-check.sh"
)

run_check() {
  local label="$1"
  shift

  echo "==> $label"
  if "$@"; then
    echo "PASS: $label"
  else
    echo "FAIL: $label"
    failures=$((failures + 1))
  fi
  echo
}

echo "Bubbles Framework Validation"
echo "Repository: $REPO_ROOT"
echo

run_check "Portable surface agnosticity" bash "$SCRIPT_DIR/agnosticity-lint.sh" --quiet "${agnosticity_targets[@]}"
run_check "Workflow registry consistency" bash "$SCRIPT_DIR/workflow-registry-consistency.sh" --quiet
run_check "Mode resolver validate" bash "$SCRIPT_DIR/mode-resolver.sh" --validate
run_check "Mode resolver selftest" bash "$SCRIPT_DIR/mode-resolver-selftest.sh"
if [[ -d "$REPO_ROOT/agents" ]]; then
  agents_dir="$REPO_ROOT/agents"
else
  agents_dir="$REPO_ROOT/.github/agents"
fi
run_check "Instruction budget lint" bash "$SCRIPT_DIR/instruction-budget-lint.sh" "$agents_dir"
run_check "Agent ownership lint" bash "$SCRIPT_DIR/agent-ownership-lint.sh"
run_check "Action risk registry lint" bash "$SCRIPT_DIR/action-risk-registry-lint.sh"
run_check "Capability ledger selftest" bash "$SCRIPT_DIR/capability-ledger-selftest.sh"
run_check "Capability freshness selftest" bash "$SCRIPT_DIR/capability-freshness-selftest.sh"
run_check "Competitive docs selftest" bash "$SCRIPT_DIR/competitive-docs-selftest.sh"
run_check "Interop apply selftest" bash "$SCRIPT_DIR/interop-apply-selftest.sh"
run_check "Release manifest freshness" bash "$SCRIPT_DIR/generate-release-manifest.sh" --check
run_check "Release manifest selftest" bash "$SCRIPT_DIR/release-manifest-selftest.sh"
run_check "Install provenance selftest" bash "$SCRIPT_DIR/install-provenance-selftest.sh"
run_check "Trust doctor selftest" bash "$SCRIPT_DIR/trust-doctor-selftest.sh"
run_check "Finding closure selftest" bash "$SCRIPT_DIR/finding-closure-selftest.sh"
run_check "Super surface selftest" bash "$SCRIPT_DIR/super-surface-selftest.sh"
run_check "Workflow delegation selftest" bash "$SCRIPT_DIR/workflow-delegation-selftest.sh"
run_check "Continuation routing selftest" bash "$SCRIPT_DIR/continuation-routing-selftest.sh"
run_check "Workflow planning provenance selftest" bash "$SCRIPT_DIR/workflow-planning-provenance-selftest.sh"
run_check "Transition guard selftest" bash "$SCRIPT_DIR/state-transition-guard-selftest.sh"
run_check "Batch promotion lint selftest" bash "$SCRIPT_DIR/batch-promotion-lint-selftest.sh"
run_check "Done-spec audit selftest" bash "$SCRIPT_DIR/done-spec-audit-selftest.sh"

if [[ -x "$SCRIPT_DIR/runtime-lease-selftest.sh" ]]; then
  run_check "Runtime lease selftest" bash "$SCRIPT_DIR/runtime-lease-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/context-compactor-selftest.sh" ]]; then
  run_check "Context compactor selftest" bash "$SCRIPT_DIR/context-compactor-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/state-snapshot-selftest.sh" ]]; then
  run_check "State snapshot selftest" bash "$SCRIPT_DIR/state-snapshot-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/edit-lint-gate-selftest.sh" ]]; then
  run_check "Edit lint gate selftest" bash "$SCRIPT_DIR/edit-lint-gate-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/gate-id-grep-selftest.sh" ]]; then
  run_check "Gate ID grep selftest" bash "$SCRIPT_DIR/gate-id-grep-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/workflow-surface-selftest.sh" ]]; then
  run_check "Workflow surface selftest" bash "$SCRIPT_DIR/workflow-surface-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/agent-ownership-lint-selftest.sh" ]]; then
  run_check "Agent ownership lint selftest" bash "$SCRIPT_DIR/agent-ownership-lint-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/agnosticity-lint-selftest.sh" ]]; then
  run_check "Agnosticity lint selftest" bash "$SCRIPT_DIR/agnosticity-lint-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/artifact-freshness-guard-selftest.sh" ]]; then
  run_check "Artifact freshness guard selftest" bash "$SCRIPT_DIR/artifact-freshness-guard-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/instruction-budget-lint-selftest.sh" ]]; then
  run_check "Instruction budget lint selftest" bash "$SCRIPT_DIR/instruction-budget-lint-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/regression-baseline-guard-selftest.sh" ]]; then
  run_check "Regression baseline guard selftest" bash "$SCRIPT_DIR/regression-baseline-guard-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/regression-quality-guard-selftest.sh" ]]; then
  run_check "Regression quality guard selftest" bash "$SCRIPT_DIR/regression-quality-guard-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/traceability-guard-selftest.sh" ]]; then
  run_check "Traceability guard selftest" bash "$SCRIPT_DIR/traceability-guard-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/value-selection-lint-selftest.sh" ]]; then
  run_check "Value selection lint selftest" bash "$SCRIPT_DIR/value-selection-lint-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/governance-index-lint-selftest.sh" ]]; then
  run_check "Governance index lint selftest" bash "$SCRIPT_DIR/governance-index-lint-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/orchestrator-tool-frontmatter-lint-selftest.sh" ]]; then
  run_check "Orchestrator tool frontmatter lint selftest" bash "$SCRIPT_DIR/orchestrator-tool-frontmatter-lint-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/trajectory-inspector-selftest.sh" ]]; then
  run_check "Trajectory inspector selftest" bash "$SCRIPT_DIR/trajectory-inspector-selftest.sh"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "Framework validation failed with $failures failing check(s)."
  exit 1
fi

echo "Framework validation passed."