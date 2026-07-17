#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/guard-lib.sh"
if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
else
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

# macOS portability shim. BSD userland diverges from GNU coreutils on `sed -i`
# (BSD needs `sed -i ''`) and lacks `timeout` (coreutils ships `gsed`/`gtimeout`).
# Several selftests below invoke `sed -i` / `timeout` in GNU form. When the GNU
# binaries are present under their `g`-prefixed names, expose them as plain `sed`
# / `timeout` on PATH for THIS process and every selftest subprocess it spawns,
# so the whole validation runs unchanged on Linux + macOS. On Linux (unprefixed
# GNU tools already present) every probe short-circuits and this is a no-op.
_bubbles_compat_dir=""
if ! sed --version >/dev/null 2>&1 && command -v gsed >/dev/null 2>&1; then
  _bubbles_compat_dir="$(mktemp -d)"
  ln -sf "$(command -v gsed)" "$_bubbles_compat_dir/sed"
fi
if ! command -v timeout >/dev/null 2>&1 && command -v gtimeout >/dev/null 2>&1; then
  [[ -n "$_bubbles_compat_dir" ]] || _bubbles_compat_dir="$(mktemp -d)"
  ln -sf "$(command -v gtimeout)" "$_bubbles_compat_dir/timeout"
fi
if [[ -n "$_bubbles_compat_dir" ]]; then
  PATH="$_bubbles_compat_dir:$PATH"
  export PATH
  trap 'rm -rf "$_bubbles_compat_dir"' EXIT
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
declare -a failed_check_labels=()

# IMP-012 tiering (opt-in, non-breaking). Default tier=full runs EVERY check
# exactly as before. `--tier=core` runs only the fast, high-signal structural
# subset (registry/lint/generator/scan selftests) for a quick local signal;
# the pre-push / release-check path passes no flag, so it is unchanged.
# `--list-tier=core` DRY-LISTS which checks the core tier would run/skip and
# exits 0 (no execution) — used by the tiering selftest and by operators.
VALIDATE_TIER="${BUBBLES_VALIDATE_TIER:-full}"
LIST_TIER_ONLY="false"
for _arg in "$@"; do
  case "$_arg" in
    --tier=core | --tier=full) VALIDATE_TIER="${_arg#--tier=}" ;;
    --list-tier=core | --list-tier=full)
      VALIDATE_TIER="${_arg#--list-tier=}"
      LIST_TIER_ONLY="true"
      ;;
    -h | --help)
      echo "Usage: framework-validate.sh [--tier=core|full] [--list-tier=core|full]"
      echo "  (no flag)        run every check (full tier — unchanged default)"
      echo "  --tier=core      run only the fast structural/lint/generator subset"
      echo "  --list-tier=core dry-list what the core tier runs/skips, then exit 0"
      exit 0
      ;;
    *)
      echo "framework-validate: unknown argument '$_arg'." >&2
      exit 2
      ;;
  esac
done

# A check is CORE (fast, high-signal, deterministic) when its label matches one
# of these substrings. The set is intentionally small — structural registry/lint
# consistency + the cheap generator/scan selftests.
core_check_label() {
  case "$1" in
    *"Repository drift report"* | *"Gate-catalog freshness"* | \
      *"Portable surface agnosticity"* | *"Shellcheck lint"* | \
      *"Registry consistency"* | *"Gates registry"* | *"YAML schema"* | \
      *"Cheatsheet generator selftest"* | *"Modes split"* | \
      *"Scan-lib"* | *"Derived-artifact regen"* | *"Gate scaffolder"* | \
      *"drift-check selftest"* | *"hub-report selftest"*)
      return 0
      ;;
    *) return 1 ;;
  esac
}

run_check() {
  local label="$1"
  shift

  if [[ "$VALIDATE_TIER" == "core" ]] && ! core_check_label "$label"; then
    if [[ "$LIST_TIER_ONLY" == "true" ]]; then
      echo "WOULD-SKIP (non-core): $label"
    else
      echo "==> $label"
      echo "SKIP: $label (tier=core)"
      skipped=$((skipped + 1))
      echo
    fi
    return 0
  fi

  if [[ "$LIST_TIER_ONLY" == "true" ]]; then
    echo "WOULD-RUN: $label"
    return 0
  fi

  echo "==> $label"
  if "$@"; then
    echo "PASS: $label"
  else
    echo "FAIL: $label"
    failures=$((failures + 1))
    failed_check_labels+=("$label")
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
run_check "Gate-catalog freshness advisory (informational, IMP-005)" bash "$SCRIPT_DIR/gate-catalog-freshness.sh" --repo-root "$REPO_ROOT"
run_check_self_only "Portable surface agnosticity" bash "$SCRIPT_DIR/agnosticity-lint.sh" --quiet
run_check_self_only "Shellcheck lint (v7.0.2, -S warning, zero findings)" bash "$SCRIPT_DIR/shellcheck-lint.sh" --quiet
run_check_self_only "Shellcheck lint selftest (v7.0.2)" bash "$SCRIPT_DIR/shellcheck-lint-selftest.sh"
run_check "Registry consistency selftest" bash "$SCRIPT_DIR/registry-consistency-selftest.sh"
run_check "YAML schema validate" bash "$SCRIPT_DIR/yaml-schema-validate.sh"
run_check_self_only "Cheatsheet generator selftest (v6.0 / B7)" bash "$SCRIPT_DIR/generate-cheatsheet-selftest.sh"
run_check_self_only "Agent roster coverage (v7.18.0)" bash "$SCRIPT_DIR/agent-roster-coverage.sh" --repo-root "$REPO_ROOT"
run_check_self_only "Agent roster coverage selftest (v7.18.0)" bash "$SCRIPT_DIR/agent-roster-coverage-selftest.sh"
run_check "Tool-log selftest (v5.1 / M1)" bash "$SCRIPT_DIR/tool-log-selftest.sh"
run_check "Evidence-tool-log bridge selftest (v6.0 / B1)" bash "$SCRIPT_DIR/evidence-tool-log-bridge-selftest.sh"
run_check "Diff-evidence guard selftest (v6.0 / B2)" bash "$SCRIPT_DIR/diff-evidence-guard-selftest.sh"
run_check "Result-envelope validate selftest (v6.0 / B3)" bash "$SCRIPT_DIR/result-envelope-validate-selftest.sh"
run_check "Artifact-lint certifying-window selftest (v7.17.0)" bash "$SCRIPT_DIR/artifact-lint-selftest.sh"
run_check "Skill-evolution selftest (v7.16.0 / IMP-016)" bash "$SCRIPT_DIR/skill-evolution-selftest.sh"
run_check "Inventory parity check selftest (IMP-005)" bash "$SCRIPT_DIR/inventory-parity-check-selftest.sh"
# Live parity check is framework-source-only: skills/INVENTORY.md is a source-repo
# artifact and is not vendored into downstream install trees.
run_check_self_only "Inventory parity check (live, IMP-005)" bash "$SCRIPT_DIR/inventory-parity-check.sh" "$REPO_ROOT"
# Case-collision guard (IMP-017): the hermetic selftest PLUS a live scan of the
# repo's tracked files. The live check is deliberately NOT source-only — a
# case-only duplicate path is a defect in ANY git repo (downstream installs
# included), and the guard no-ops gracefully outside a git work tree.
run_check "Case-collision guard selftest (IMP-017)" bash "$SCRIPT_DIR/case-collision-guard-selftest.sh"
run_check "Case-collision guard (live, IMP-017)" bash "$SCRIPT_DIR/case-collision-guard.sh" --repo-root "$REPO_ROOT"
# macOS/WSL portability guard: run its HERMETIC selftest (green + one red fixture
# per class + self-portability), NOT a scan of the framework's own scripts (which
# intentionally use raw timeout/sed -i mediated by guard-lib + the PATH shim).
macos_portability_guard_timeout_seconds="${BUBBLES_MACOS_PORTABILITY_GUARD_SELFTEST_TIMEOUT_SECONDS:-120}"
run_check "macOS portability guard selftest (bubbles-cross-platform-shell)" bubbles_run_with_timeout "$macos_portability_guard_timeout_seconds" bash "$SCRIPT_DIR/macos-portability-guard-selftest.sh"
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
if [[ -x "$SCRIPT_DIR/adversarial-resolve-selftest.sh" ]]; then
  run_check "Adversarial-resolve control plane selftest (IMP-002 / S0)" bash "$SCRIPT_DIR/adversarial-resolve-selftest.sh"
fi
if [[ -f "$SCRIPT_DIR/adversarial-aggregate-selftest.sh" ]]; then
  # The selftest validates the source-only eval schema and canonical source surfaces.
  run_check_self_only "Adversarial aggregate selftest (IMP-020 / S2)" bash "$SCRIPT_DIR/adversarial-aggregate-selftest.sh"
fi
if [[ -x "$SCRIPT_DIR/control-plane-policy-activation-selftest.sh" ]]; then
  run_check "Control-plane policy-activation selftest (G055-G060 SST precedence + G060 red->green ordering)" bash "$SCRIPT_DIR/control-plane-policy-activation-selftest.sh"
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
run_check "Risk-tier resolver selftest (BFW-01 / IMP-021)" bash "$SCRIPT_DIR/risk-tier-resolve-selftest.sh"
run_check "Transition contract resolver selftest (BUG-009 S02)" bash "$SCRIPT_DIR/transition-contract-resolver-selftest.sh"
run_check "Audit result contract lint selftest (BUG-009 S04)" bash "$SCRIPT_DIR/audit-result-contract-lint-selftest.sh"
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
run_check "Workflow runner grants lint (G064)" bash "$SCRIPT_DIR/workflow-runner-grants-lint.sh"
run_check "Workflow runner grants lint selftest (G064)" bash "$SCRIPT_DIR/workflow-runner-grants-lint-selftest.sh"
if [[ -x "$SCRIPT_DIR/mcp-grant-selftest.sh" ]]; then
  # Source-only: asserts the canonical 'bubbles' MCP token; downstream installs
  # carry a per-repo 'bubbles-<slug>' token, so this can only hold in source.
  run_check_self_only "MCP tool grant selftest (v7.1)" bash "$SCRIPT_DIR/mcp-grant-selftest.sh"
fi
run_check "Action risk registry lint" bash "$SCRIPT_DIR/action-risk-registry-lint.sh"
run_check_self_only "Capability ledger selftest" bash "$SCRIPT_DIR/capability-ledger-selftest.sh"
run_check "Capability consumer freshness selftest (G127)" bash "$SCRIPT_DIR/capability-consumer-freshness-selftest.sh"
run_check_self_only "Capability consumer freshness (live, G127)" bash "$SCRIPT_DIR/capability-consumer-freshness.sh" --repo-root "$REPO_ROOT"
run_check_self_only "Capability freshness selftest" bash "$SCRIPT_DIR/capability-freshness-selftest.sh"
run_check_self_only "Competitive docs selftest" bash "$SCRIPT_DIR/competitive-docs-selftest.sh"
run_check_self_only "Interop apply selftest" bash "$SCRIPT_DIR/interop-apply-selftest.sh"
run_check_self_only "Release manifest freshness" bash "$SCRIPT_DIR/generate-release-manifest.sh" --check
run_check_self_only "Release manifest selftest" bash "$SCRIPT_DIR/release-manifest-selftest.sh"
run_check_self_only "Release manifest purity selftest" bash "$SCRIPT_DIR/release-manifest-purity-selftest.sh"
run_check_self_only "Derived-artifact regen wrapper selftest (IMP-007)" bash "$SCRIPT_DIR/regen-derived-selftest.sh"
run_check_self_only "Gate scaffolder selftest (IMP-011)" bash "$SCRIPT_DIR/scaffold-gate-selftest.sh"
run_check_self_only "Framework drift-check selftest (IMP-013)" bash "$SCRIPT_DIR/bubbles-drift-check-selftest.sh"
run_check_self_only "Governance hub-report selftest (IMP-014)" bash "$SCRIPT_DIR/bubbles-hub-report-selftest.sh"
run_check_self_only "Scan-lib helpers selftest (IMP-009)" bash "$SCRIPT_DIR/scan-lib-selftest.sh"
run_check_self_only "Framework-validate tiering selftest (IMP-012)" bash "$SCRIPT_DIR/framework-validate-tier-selftest.sh"
run_check_self_only "Install provenance selftest" bash "$SCRIPT_DIR/install-provenance-selftest.sh"
run_check_self_only "Trust doctor selftest" bash "$SCRIPT_DIR/trust-doctor-selftest.sh"
run_check "Repo-binding preflight selftest (BFW-05 / IMP-025)" bash "$SCRIPT_DIR/repo-binding-preflight-selftest.sh"
run_check "Finding closure selftest" bash "$SCRIPT_DIR/finding-closure-selftest.sh"
run_check "Super surface selftest" bash "$SCRIPT_DIR/super-surface-selftest.sh"
run_check "Workflow delegation selftest" bash "$SCRIPT_DIR/workflow-delegation-selftest.sh"
run_check "Top-level-runtime routing selftest" bash "$SCRIPT_DIR/top-level-runtime-routing-selftest.sh"
run_check "Continuation routing selftest" bash "$SCRIPT_DIR/continuation-routing-selftest.sh"
planning_provenance_timeout_seconds="${BUBBLES_WORKFLOW_PLANNING_PROVENANCE_SELFTEST_TIMEOUT_SECONDS:-120}"
run_check "Workflow planning provenance selftest" bubbles_run_with_timeout "$planning_provenance_timeout_seconds" bash "$SCRIPT_DIR/workflow-planning-provenance-selftest.sh"
run_check "Transition guard selftest" bash "$SCRIPT_DIR/state-transition-guard-selftest.sh"
run_check_self_only "BUG-009 planning audit contract regression" bash "$REPO_ROOT/tests/regression/test_23_planning_audit_contract.sh"
run_check_self_only "BUG-013 sensitive client storage regression" bash "$REPO_ROOT/tests/regression/test_24_g028_sensitive_client_storage.sh"
run_check_self_only "BUG-018 traceability Test Plan heading-depth regression" bash "$REPO_ROOT/tests/regression/test_25_traceability_test_plan_heading_depth.sh"
run_check_self_only "BUG-019 state-transition compound MJS test-path regression" bash "$REPO_ROOT/tests/regression/test_26_state_transition_spec_mjs_path.sh"
run_check_self_only "BUG-021 portable framework deadline regression" bash "$REPO_ROOT/tests/regression/test_28_framework_validate_portable_timeout.sh"
run_check "Convergence cap guard selftest" bash "$SCRIPT_DIR/convergence-cap-guard-selftest.sh"
run_check "Session cap guard selftest (G128)" bash "$SCRIPT_DIR/session-cap-guard-selftest.sh"
run_check "Session cap guard (live, G128)" env BUBBLES_REPO_ROOT="$REPO_ROOT" bash "$SCRIPT_DIR/session-cap-guard.sh" --quiet
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
run_check "Vertical-delivery plan guard selftest (BFW-02 / IMP-022)" bash "$SCRIPT_DIR/vertical-delivery-plan-guard-selftest.sh"
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

if [[ -x "$SCRIPT_DIR/release-delivery-reconciliation-guard-selftest.sh" ]]; then
  run_check "Release delivery reconciliation guard selftest (G101)" bash "$SCRIPT_DIR/release-delivery-reconciliation-guard-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/release-delivery-reconciliation-guard.sh" ]]; then
  run_check "Release delivery reconciliation guard (live, G101)" bash "$SCRIPT_DIR/release-delivery-reconciliation-guard.sh" --repo-root "$REPO_ROOT"
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

if [[ -x "$SCRIPT_DIR/prometheus-adapter-fetch-selftest.sh" ]]; then
  run_check "Prometheus adapter live-fetch selftest (P2)" bash "$SCRIPT_DIR/prometheus-adapter-fetch-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/observability-posture-guard-selftest.sh" ]]; then
  # Source-only: drives source-tree fixtures under tests/fixtures/observability/
  # which the installer does not vendor downstream. The live G098 guard below
  # still runs everywhere.
  run_check_self_only "Observability posture guard selftest (G098)" bash "$SCRIPT_DIR/observability-posture-guard-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/observability-opt-out-guard-selftest.sh" ]]; then
  # Source-only: drives source-tree observability fixtures (not vendored
  # downstream). The live G099 guard below still runs everywhere.
  run_check_self_only "Observability opt-out guard selftest (G099)" bash "$SCRIPT_DIR/observability-opt-out-guard-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/observability-posture-guard.sh" ]]; then
  run_check "Observability posture guard (live, G098)" bash "$SCRIPT_DIR/observability-posture-guard.sh" --repo-root "$REPO_ROOT"
fi

if [[ -x "$SCRIPT_DIR/observability-opt-out-guard.sh" ]]; then
  run_check "Observability opt-out guard (live, G099)" bash "$SCRIPT_DIR/observability-opt-out-guard.sh" --repo-root "$REPO_ROOT"
fi

if [[ -x "$SCRIPT_DIR/observability-slo-guard-selftest.sh" ]]; then
  # Source-only: drives source-tree observability fixtures (not vendored
  # downstream). The live G100 guard below still runs everywhere.
  run_check_self_only "Observability SLO guard selftest (G100)" bash "$SCRIPT_DIR/observability-slo-guard-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/observability-slo-guard.sh" ]]; then
  run_check "Observability SLO guard (live, G100)" bash "$SCRIPT_DIR/observability-slo-guard.sh" --repo-root "$REPO_ROOT"
fi

if [[ -x "$SCRIPT_DIR/observability-endpoint-resolve-selftest.sh" ]]; then
  run_check "Observability endpoint resolver selftest (SCOPE-3)" bash "$SCRIPT_DIR/observability-endpoint-resolve-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/observability-check-selftest.sh" ]]; then
  # Source-only: drives source-tree observability fixtures (not vendored
  # downstream). The live check twin below still runs everywhere.
  run_check_self_only "Observability check twin selftest (wired fixture)" bash "$SCRIPT_DIR/observability-check-selftest.sh"
fi

if [[ -x "$SCRIPT_DIR/observability-check.sh" ]]; then
  run_check "Observability check twin (live, posture+SLO+trace+endpoints)" bash "$SCRIPT_DIR/observability-check.sh" --repo-root "$REPO_ROOT"
fi

if [[ -x "$SCRIPT_DIR/env-pollution-scan-selftest.sh" ]]; then
  run_check "Env pollution scan selftest (G115)" bash "$SCRIPT_DIR/env-pollution-scan-selftest.sh"
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

if [[ "$LIST_TIER_ONLY" == "true" ]]; then
  echo "Tier listing complete (tier=$VALIDATE_TIER). No checks were executed."
  exit 0
fi

if [[ "$failures" -gt 0 ]]; then
  echo "Framework validation failed with $failures failing check(s)$([[ "$skipped" -gt 0 ]] && echo " ($skipped self-only check(s) skipped under install-mode=$INSTALL_MODE)")."
  echo "Failed checks:"
  for failed_label in "${failed_check_labels[@]}"; do
    echo "  - $failed_label"
  done
  exit 1
fi

if [[ "$skipped" -gt 0 ]]; then
  echo "Framework validation passed ($skipped self-only check(s) skipped under install-mode=$INSTALL_MODE). Run from a framework-source tree to execute them."
fi

echo "Framework validation passed."