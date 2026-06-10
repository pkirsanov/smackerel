#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
else
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

# v5.3 / G1: install-mode detection. Many selftests below were authored
# inside the framework source repo and assume `install.sh`, `VERSION`, or
# the framework's own `README.md`/`docs/` layout are present. From a
# downstream install tree (which only carries `.github/bubbles/...`), those
# assertions cannot hold. Detect the install mode once here and use it to
# drive run_check_self_only below.
#
# Override: BUBBLES_FRAMEWORK_VALIDATE_MODE=source|downstream forces a mode
# (useful for selftests that synthesize either tree).
INSTALL_MODE="${BUBBLES_FRAMEWORK_VALIDATE_MODE:-}"
if [[ -z "$INSTALL_MODE" ]]; then
  if [[ -f "$REPO_ROOT/install.sh" && -f "$REPO_ROOT/VERSION" && -f "$REPO_ROOT/bubbles/scripts/cli.sh" ]]; then
    INSTALL_MODE="source"
  elif [[ -f "$REPO_ROOT/.github/bubbles/.install-source.json" ]]; then
    INSTALL_MODE="downstream"
  else
    INSTALL_MODE="unknown"
  fi
fi

failures=0
skipped=0

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

# Wrapper for selftests that only make sense when run inside the framework
# source tree (those that invoke install.sh, walk VERSION, or assert the
# framework's own README/docs layout). When INSTALL_MODE != "source", emit
# a SKIP line instead of running them so downstream framework-validate
# exits 0 with explicit accounting instead of FAIL'ing on
# expected-to-be-missing files.
run_check_self_only() {
  local label="$1"
  shift

  if [[ "$INSTALL_MODE" != "source" ]]; then
    echo "==> $label"
    echo "SKIP: $label (framework-source-only; install-mode=$INSTALL_MODE)"
    skipped=$((skipped + 1))
    echo
    return 0
  fi
  run_check "$label" "$@"
}

echo "Bubbles Framework Validation"
echo "Repository: $REPO_ROOT"
echo "Install mode: $INSTALL_MODE"
echo

run_check "Repository drift report (informational)" bash "$SCRIPT_DIR/repo-drift-report.sh" --repo-root "$REPO_ROOT"
run_check_self_only "Portable surface agnosticity" bash "$SCRIPT_DIR/agnosticity-lint.sh" --quiet "${agnosticity_targets[@]}"
run_check_self_only "Shellcheck lint (v7.0.2, -S warning, zero findings)" bash "$SCRIPT_DIR/shellcheck-lint.sh" --quiet
run_check_self_only "Shellcheck lint selftest (v7.0.2)" bash "$SCRIPT_DIR/shellcheck-lint-selftest.sh"
run_check "Registry consistency selftest" bash "$SCRIPT_DIR/registry-consistency-selftest.sh"
run_check "YAML schema validate" bash "$SCRIPT_DIR/yaml-schema-validate.sh"
run_check_self_only "Cheatsheet generator selftest (v6.0 / B7)" bash "$SCRIPT_DIR/generate-cheatsheet-selftest.sh"
run_check "Tool-log selftest (v5.1 / M1)" bash "$SCRIPT_DIR/tool-log-selftest.sh"
run_check "Evidence-tool-log bridge selftest (v6.0 / B1)" bash "$SCRIPT_DIR/evidence-tool-log-bridge-selftest.sh"
run_check "Diff-evidence guard selftest (v6.0 / B2)" bash "$SCRIPT_DIR/diff-evidence-guard-selftest.sh"
run_check "Result-envelope validate selftest (v6.0 / B3)" bash "$SCRIPT_DIR/result-envelope-validate-selftest.sh"
run_check_self_only "Installer manifest check (v6.0 / B9)" bash "$SCRIPT_DIR/generate-installer.sh"
run_check_self_only "Installer manifest selftest (v6.0 / B9)" bash "$SCRIPT_DIR/generate-installer-selftest.sh"
if [[ -x "$SCRIPT_DIR/migrate-modes-v5-to-v6.sh" ]]; then
  run_check_self_only "Migrate-modes-v5-to-v6 selftest (v6.0 / C1)" bash "$SCRIPT_DIR/migrate-modes-v5-to-v6-selftest.sh"
fi
run_check "Gates registry drift (v5.2 / F4)" bash "$SCRIPT_DIR/generate-gates-block.sh" --check
if [[ -x "$SCRIPT_DIR/generate-modes-block.sh" ]]; then
  run_check "Modes split no-duplication (v6.1 / S2)" bash "$SCRIPT_DIR/generate-modes-block.sh" --check
fi
run_check "Gates registry selftest (v5.2 / F4)" bash "$SCRIPT_DIR/gates-registry-selftest.sh"
if [[ -x "$SCRIPT_DIR/mode-family-inventory-selftest.sh" ]]; then
  run_check "Mode-family inventory selftest (v6.1 / R5)" bash "$SCRIPT_DIR/mode-family-inventory-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/model-tier-advisory-selftest.sh" ]]; then
  run_check "Model-tier floor selftest (v6.1 / S9 / G126)" bash "$SCRIPT_DIR/model-tier-advisory-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/parallel-fanout-determinism-selftest.sh" ]]; then
  run_check "Parallel fan-out determinism selftest (v6.1 / B10 / R8)" bash "$SCRIPT_DIR/parallel-fanout-determinism-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/pre-tool-risk-gate-selftest.sh" ]]; then
  run_check "Pre-tool risk gate selftest (v6.1 / R10)" bash "$SCRIPT_DIR/pre-tool-risk-gate-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/tool-capture-shim-selftest.sh" ]]; then
  run_check "Tool-capture shim selftest (v6.1 / R2)" bash "$SCRIPT_DIR/tool-capture-shim-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/eval-harness-selftest.sh" ]]; then
  run_check_self_only "Golden-task eval harness selftest (v6.1 / R11)" bash "$SCRIPT_DIR/eval-harness-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/state-transition-guard-perf-selftest.sh" ]]; then
  run_check "Guard reliability perf selftest (v6.1 / R1 / BUG-001)" bash "$SCRIPT_DIR/state-transition-guard-perf-selftest.sh"
fi
run_check "Result-envelope validate (v6.0 / B3, malformed blocks)" bash "$SCRIPT_DIR/result-envelope-validate.sh"
run_check "v5.2 aggregate selftest (F1, F3, F6, F7)" bash "$SCRIPT_DIR/v5.2-selftest.sh"
if [[ -x "$SCRIPT_DIR/v5.3-selftest.sh" ]]; then
  run_check "v5.3 downstream-install selftest (G1)" bash "$SCRIPT_DIR/v5.3-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/mcp-server-selftest.sh" ]]; then
  run_check "v6 MCP server selftest (A5)" bash "$SCRIPT_DIR/mcp-server-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/mcp-http-transport-selftest.sh" ]]; then
  run_check "MCP HTTP transport selftest (v6.1 / R9)" bash "$SCRIPT_DIR/mcp-http-transport-selftest.sh"
fi
run_check "Workflow registry consistency" bash "$SCRIPT_DIR/workflow-registry-consistency.sh" --quiet
run_check "Mode resolver validate" bash "$SCRIPT_DIR/mode-resolver.sh" --validate
run_check "Mode resolver selftest" bash "$SCRIPT_DIR/mode-resolver-selftest.sh"
run_check "Mode alias selftest (v6.0 / B4)" bash "$SCRIPT_DIR/mode-alias-selftest.sh"
if [[ -x "$SCRIPT_DIR/v7-selftest.sh" ]]; then
  run_check "v7 mode-name removal + grandfather selftest (v7.0)" bash "$SCRIPT_DIR/v7-selftest.sh"
fi
run_check "Spec-review handoff selftest" bash "$SCRIPT_DIR/spec-review-handoff-selftest.sh"
if [[ -d "$REPO_ROOT/agents" ]]; then
  agents_dir="$REPO_ROOT/agents"
else
  agents_dir="$REPO_ROOT/.github/agents"
fi
run_check "Instruction budget lint" bash "$SCRIPT_DIR/instruction-budget-lint.sh" "$agents_dir"
run_check "Agent ownership lint" bash "$SCRIPT_DIR/agent-ownership-lint.sh"
run_check "Orchestrator tool frontmatter lint (v7.0.3)" bash "$SCRIPT_DIR/orchestrator-tool-frontmatter-lint.sh"
if [[ -x "$SCRIPT_DIR/mcp-grant-selftest.sh" ]]; then
  run_check "MCP tool grant selftest (v7.1)" bash "$SCRIPT_DIR/mcp-grant-selftest.sh"
fi
run_check "Action risk registry lint" bash "$SCRIPT_DIR/action-risk-registry-lint.sh"
run_check_self_only "Capability ledger selftest" bash "$SCRIPT_DIR/capability-ledger-selftest.sh"
run_check_self_only "Capability freshness selftest" bash "$SCRIPT_DIR/capability-freshness-selftest.sh"
run_check_self_only "Competitive docs selftest" bash "$SCRIPT_DIR/competitive-docs-selftest.sh"
run_check_self_only "Interop apply selftest" bash "$SCRIPT_DIR/interop-apply-selftest.sh"
run_check_self_only "Release manifest freshness" bash "$SCRIPT_DIR/generate-release-manifest.sh" --check
run_check_self_only "Release manifest selftest" bash "$SCRIPT_DIR/release-manifest-selftest.sh"
run_check_self_only "Release manifest purity selftest" bash "$SCRIPT_DIR/release-manifest-purity-selftest.sh"
run_check_self_only "Install provenance selftest" bash "$SCRIPT_DIR/install-provenance-selftest.sh"
run_check_self_only "Trust doctor selftest" bash "$SCRIPT_DIR/trust-doctor-selftest.sh"
run_check "Finding closure selftest" bash "$SCRIPT_DIR/finding-closure-selftest.sh"
run_check "Super surface selftest" bash "$SCRIPT_DIR/super-surface-selftest.sh"
run_check "Workflow delegation selftest" bash "$SCRIPT_DIR/workflow-delegation-selftest.sh"
run_check "Top-level-runtime routing selftest" bash "$SCRIPT_DIR/top-level-runtime-routing-selftest.sh"
run_check "Continuation routing selftest" bash "$SCRIPT_DIR/continuation-routing-selftest.sh"
planning_provenance_timeout_seconds="${BUBBLES_WORKFLOW_PLANNING_PROVENANCE_SELFTEST_TIMEOUT_SECONDS:-120}"
run_check "Workflow planning provenance selftest" timeout "$planning_provenance_timeout_seconds" bash "$SCRIPT_DIR/workflow-planning-provenance-selftest.sh"
run_check "Transition guard selftest" bash "$SCRIPT_DIR/state-transition-guard-selftest.sh"
run_check "Convergence cap guard selftest" bash "$SCRIPT_DIR/convergence-cap-guard-selftest.sh"
run_check "Compaction discipline guard selftest" bash "$SCRIPT_DIR/compaction-discipline-guard-selftest.sh"
run_check "Pre-existing deferral guard selftest" bash "$SCRIPT_DIR/pre-existing-deferral-guard-selftest.sh"
run_check "Discovered-issue disposition guard selftest (G095)" bash "$SCRIPT_DIR/discovered-issue-disposition-guard-selftest.sh"
run_check "Requirement-mechanism guard selftest (G097)" bash "$SCRIPT_DIR/requirement-mechanism-guard-selftest.sh"
run_check "Framework dogfood guard selftest" bash "$SCRIPT_DIR/framework-dogfood-guard-selftest.sh"
run_check "Orchestrator persistence lint selftest" bash "$SCRIPT_DIR/orchestrator-persistence-lint-selftest.sh"
run_check "Validation latency report selftest" bash "$SCRIPT_DIR/validation-latency-report-selftest.sh"
run_check "Retro convergence health selftest" bash "$SCRIPT_DIR/retro-convergence-health-selftest.sh"
run_check "Planning workflow chain guard selftest" bash "$SCRIPT_DIR/planning-workflow-chain-guard-selftest.sh"
run_check "Capability foundation guard selftest" bash "$SCRIPT_DIR/capability-foundation-guard-selftest.sh"
run_check "State linkage backfill selftest" bash "$SCRIPT_DIR/state-linkage-backfill-selftest.sh"
run_check "Planning packet linkage guard selftest" bash "$SCRIPT_DIR/planning-packet-linkage-guard-selftest.sh"
run_check "Post-certification spec edit guard selftest" bash "$SCRIPT_DIR/post-cert-spec-edit-guard-selftest.sh"
run_check "Inter-spec dependency guard selftest" bash "$SCRIPT_DIR/inter-spec-dependency-guard-selftest.sh"
run_check "Strict terminal status guard selftest" bash "$SCRIPT_DIR/strict-terminal-status-guard-selftest.sh"
run_check "Delivery implementation delta guard selftest" bash "$SCRIPT_DIR/delivery-implementation-delta-guard-selftest.sh"
run_check "Batch promotion lint selftest" bash "$SCRIPT_DIR/batch-promotion-lint-selftest.sh"
run_check "Done-spec audit selftest" bash "$SCRIPT_DIR/done-spec-audit-selftest.sh"
run_check "Test impact plan selftest" bash "$SCRIPT_DIR/test-impact-plan-selftest.sh"
run_check "Trace contract guard selftest" bash "$SCRIPT_DIR/trace-contract-guard-selftest.sh"

if [[ -x "$SCRIPT_DIR/runtime-lease-selftest.sh" ]]; then
  run_check_self_only "Runtime lease selftest" bash "$SCRIPT_DIR/runtime-lease-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/context-compactor-selftest.sh" ]]; then
  run_check "Context compactor selftest" bash "$SCRIPT_DIR/context-compactor-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/state-snapshot-selftest.sh" ]]; then
  run_check "State snapshot selftest" bash "$SCRIPT_DIR/state-snapshot-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/implementation-reality-scan-selftest.sh" ]]; then
  run_check "Implementation reality scan selftest" bash "$SCRIPT_DIR/implementation-reality-scan-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/edit-lint-gate-selftest.sh" ]]; then
  run_check "Edit lint gate selftest" bash "$SCRIPT_DIR/edit-lint-gate-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/gate-id-grep-selftest.sh" ]]; then
  run_check "Gate ID grep selftest" bash "$SCRIPT_DIR/gate-id-grep-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/release-packet-location-guard-selftest.sh" ]]; then
  run_check "Release packet location guard selftest" bash "$SCRIPT_DIR/release-packet-location-guard-selftest.sh"
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

if [[ -x "$SCRIPT_DIR/propagation-policy-guard-selftest.sh" ]]; then
  run_check "Propagation policy guard selftest" bash "$SCRIPT_DIR/propagation-policy-guard-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/release-train-rollup-selftest.sh" ]]; then
  run_check "Release train rollup selftest" bash "$SCRIPT_DIR/release-train-rollup-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/observability-adapter-lint-selftest.sh" ]]; then
  run_check "Observability adapter lint selftest" bash "$SCRIPT_DIR/observability-adapter-lint-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/observability-adapter-lint.sh" && -d "$REPO_ROOT/bubbles/adapters/observability" ]]; then
  run_check "Observability adapter lint (live)" bash "$SCRIPT_DIR/observability-adapter-lint.sh" "$REPO_ROOT"
fi

if [[ -x "$SCRIPT_DIR/scenario-compile-lint-selftest.sh" ]]; then
  run_check "Scenario compile lint selftest" bash "$SCRIPT_DIR/scenario-compile-lint-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/retro-framework-health-selftest.sh" ]]; then
  run_check "Retro framework-health selftest" bash "$SCRIPT_DIR/retro-framework-health-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/intent-routes-lint-selftest.sh" ]]; then
  run_check "Intent routes lint selftest" bash "$SCRIPT_DIR/intent-routes-lint-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/intent-routes-lint.sh" && -f "$REPO_ROOT/bubbles/intent-routes.yaml" ]]; then
  run_check "Intent routes lint (live)" bash "$SCRIPT_DIR/intent-routes-lint.sh" "$REPO_ROOT"
fi

if [[ -x "$SCRIPT_DIR/stale-deferral-lint-selftest.sh" ]]; then
  run_check "Stale-deferral lint selftest" bash "$SCRIPT_DIR/stale-deferral-lint-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/stale-deferral-lint.sh" ]]; then
  # Live scan is source-only: it reads the framework VERSION and scans the
  # framework's own managed docs. Downstream repos have their own VERSION +
  # product docs, so the lapsed-promise comparison is meaningful only here.
  run_check_self_only "Stale-deferral lint (live)" bash "$SCRIPT_DIR/stale-deferral-lint.sh" "$REPO_ROOT"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "Framework validation failed with $failures failing check(s)$([[ "$skipped" -gt 0 ]] && echo " ($skipped self-only check(s) skipped under install-mode=$INSTALL_MODE)")."
  exit 1
fi

if [[ "$skipped" -gt 0 ]]; then
  echo "Framework validation passed ($skipped self-only check(s) skipped under install-mode=$INSTALL_MODE). Run from a framework-source tree to execute them."
fi

echo "Framework validation passed."