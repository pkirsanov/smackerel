#!/usr/bin/env bash
# =============================================================================
# state-transition-guard.sh
# =============================================================================
# MANDATORY guard script that MUST be executed BEFORE any state.json status
# transition to "done". This is the mechanical enforcement layer that prevents
# agents from fabricating completion status.
#
# Usage:
#   bash bubbles/scripts/state-transition-guard.sh <feature-dir> [--revert-on-fail]
#
# Exit codes:
#   0 = All checks pass, transition to "done" is permitted
#   1 = One or more checks failed, transition BLOCKED
#   2 = Usage error / missing arguments
#
# When --revert-on-fail is specified and checks fail, the script automatically
# reverts the top-level and certification status to "in_progress" and clears
# stale completion arrays (`completedScopes`, `certifiedCompletedPhases`,
# `completedPhaseClaims`, and legacy `completedPhases`).
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source fun mode support
source "$SCRIPT_DIR/fun-mode.sh"

# ─────────────────────────────────────────────────────────────────────────────
# BUG-001 reliability helpers (R1), extracted to guard-lib.sh as the first step
# of the guard split. bubbles_run_with_timeout / bubbles_pruned_find convert
# hangs (untimed sub-guards, unbounded whole-repo find walks over .git /
# node_modules / target / build caches) into bounded, observable failures.
# ─────────────────────────────────────────────────────────────────────────────
source "$SCRIPT_DIR/guard-lib.sh"

# Shared scan helpers (IMP-009): bubbles_status_lines centralizes the BUG-006
# blockquote exclusion used by Check 4B + Check 5 so they stay in lockstep.
source "$SCRIPT_DIR/scan-lib.sh"

feature_dir="${1:-}"
revert_on_fail="false"

for arg in "$@"; do
  if [[ "$arg" == "--revert-on-fail" ]]; then
    revert_on_fail="true"
  fi
done

if [[ -z "$feature_dir" ]]; then
  echo "ERROR: missing feature directory argument"
  echo "Usage: bash bubbles/scripts/state-transition-guard.sh specs/<NNN-feature-name> [--revert-on-fail]"
  exit 2
fi

if [[ ! -d "$feature_dir" ]]; then
  echo "ERROR: feature directory not found: $feature_dir"
  exit 2
fi

resolve_script_repo_root() {
  if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
    (cd "$SCRIPT_DIR/../../.." && pwd -P)
  else
    (cd "$SCRIPT_DIR/../.." && pwd -P)
  fi
}

resolve_feature_repo_root() {
  local feature_abs parent git_repo_root=""

  feature_abs="$(cd "$feature_dir" && pwd -P)"
  parent="$(dirname "$feature_abs")"
  if [[ "$(basename "$parent")" == "specs" ]]; then
    (cd "$(dirname "$parent")" && pwd -P)
    return 0
  fi

  if command -v git >/dev/null 2>&1 && git -C "$feature_dir" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    git_repo_root="$(git -C "$feature_dir" rev-parse --show-toplevel 2>/dev/null || true)"
  fi
  if [[ -n "$git_repo_root" ]]; then
    (cd "$git_repo_root" && pwd -P)
    return 0
  fi

  resolve_script_repo_root
}

script_repo_root="$(resolve_script_repo_root)"
guard_repo_root="$(resolve_feature_repo_root)"
feature_abs="$(cd "$feature_dir" && pwd -P)"

resolve_workflow_registry_file() {
  local candidate
  for candidate in \
    "$guard_repo_root/bubbles/workflows.yaml" \
    "$guard_repo_root/.github/bubbles/workflows.yaml" \
    "$script_repo_root/bubbles/workflows.yaml" \
    "$script_repo_root/.github/bubbles/workflows.yaml"; do
    if [[ -f "$candidate" ]]; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done
  return 1
}

workflow_registry_file="$(resolve_workflow_registry_file || true)"
# v6.1 (S2 true split): mode definitions live in bubbles/workflows/modes.yaml,
# beside workflows.yaml. Prefer it for the raw modes: awk parse below; fall
# back to workflows.yaml for pre-split repos that still embed an inline modes:
# block. (The mode-resolver fallback path also composes modes.yaml on its own.)
workflow_modes_file=""
if [[ -n "$workflow_registry_file" ]]; then
  workflow_modes_file="$(dirname "$workflow_registry_file")/workflows/modes.yaml"
  [[ -f "$workflow_modes_file" ]] || workflow_modes_file="$workflow_registry_file"
fi
is_test_fixture_dir="false"
case "$feature_abs" in
  "$guard_repo_root/tests/fixtures/"*|"$script_repo_root/tests/fixtures/"*)
    is_test_fixture_dir="true"
    ;;
esac

fixture_gate_skip() {
  local gate_name="$1"
  if [[ "$is_test_fixture_dir" == "true" ]]; then
    info "Fixture target under tests/fixtures; $gate_name is not evaluated for artifact-state fixture acceptance"
    return 0
  fi
  return 1
}

run_guard_in_feature_repo() {
  BUBBLES_REPO_ROOT="$guard_repo_root" "$@"
}

run_guard_in_script_repo() {
  BUBBLES_REPO_ROOT="$script_repo_root" "$@"
}

failures=0
warnings=0

fail() {
  local message="$1"
  echo "🔴 BLOCK: $message"
  fun_fail
  failures=$((failures + 1))
}

warn() {
  local message="$1"
  echo "⚠️  WARN: $message"
  fun_warn
  warnings=$((warnings + 1))
}

pass() {
  local message="$1"
  echo "✅ PASS: $message"
}

info() {
  local message="$1"
  echo "ℹ️  INFO: $message"
}

json_first_string() {
  local key="$1"
  local file="$2"
  if [[ ! -f "$file" ]]; then
    return 0
  fi

  grep -Eo '"'"$key"'"[[:space:]]*:[[:space:]]*"[^"]+"' "$file" \
    | head -n 1 \
    | sed -E 's/.*"'"$key"'"[[:space:]]*:[[:space:]]*"([^"]+)"/\1/'
}

json_first_bool() {
  local key="$1"
  local file="$2"
  if [[ ! -f "$file" ]]; then
    return 0
  fi

  grep -Eo '"'"$key"'"[[:space:]]*:[[:space:]]*(true|false)' "$file" \
    | head -n 1 \
    | sed -E 's/.*"'"$key"'"[[:space:]]*:[[:space:]]*(true|false)/\1/'
}

resolve_workflow_status_ceiling_from_registry() {
  local workflow_mode="$1"
  local status_ceiling=""

  [[ -n "$workflow_mode" ]] || return 1
  [[ -n "$workflow_modes_file" && -f "$workflow_modes_file" ]] || return 1

  status_ceiling="$(awk -v mode="$workflow_mode" '
    /^modes:[[:space:]]*$/ { in_modes = 1; next }
    in_modes && /^[A-Za-z0-9_-]+:[[:space:]]*$/ { exit }
    in_modes && $0 ~ "^  " mode ":[[:space:]]*$" { in_mode = 1; next }
    in_mode && $0 ~ "^  [[:alnum:]_-]+:[[:space:]]*$" { exit }
    in_mode {
      line = $0
      sub(/^[[:space:]]+/, "", line)
      if (line ~ /^statusCeiling:[[:space:]]*/) {
        sub(/^statusCeiling:[[:space:]]*/, "", line)
        gsub(/"/, "", line)
        print line
        exit
      }
    }
  ' "$workflow_modes_file")"

  [[ -n "$status_ceiling" ]] || return 1
  printf '%s\n' "$status_ceiling"
}

resolve_workflow_status_ceiling() {
  local workflow_mode="$1"
  local resolver="$SCRIPT_DIR/mode-resolver.sh"
  local resolved=""
  local status_ceiling=""

  [[ -n "$workflow_mode" ]] || return 1
  [[ -f "$resolver" ]] || return 1

  status_ceiling="$(resolve_workflow_status_ceiling_from_registry "$workflow_mode" || true)"
  if [[ -z "$status_ceiling" ]]; then
    # v7: this resolves a PERSISTED workflowMode from an existing artifact, so
    # grandfather the (possibly v5-name) registry key — the resolver rejects
    # bare v5 names only for NEW operator input, not for stored modes.
    if [[ -n "$workflow_registry_file" ]]; then
      resolved="$(BUBBLES_MODE_GRANDFATHER=1 BUBBLES_WORKFLOWS_FILE="$workflow_registry_file" bash "$resolver" "$workflow_mode" 2>/dev/null || true)"
    else
      resolved="$(BUBBLES_MODE_GRANDFATHER=1 bash "$resolver" "$workflow_mode" 2>/dev/null || true)"
    fi
    status_ceiling="$(printf '%s\n' "$resolved" | awk -F':[[:space:]]*' '$1 == "statusCeiling" { gsub(/"/, "", $2); print $2; exit }')"
  fi
  [[ -n "$status_ceiling" ]] || return 1
  printf '%s\n' "$status_ceiling"
}

json_nested_string() {
  local parent_key="$1"
  local child_key="$2"
  local file="$3"
  if [[ ! -f "$file" ]]; then
    return 0
  fi

  python3 - "$file" "$parent_key" "$child_key" <<'PY'
import json
import sys

file_path, parent_key, child_key = sys.argv[1:4]
with open(file_path, encoding="utf-8") as handle:
    data = json.load(handle)

parent = data.get(parent_key, {})
value = parent.get(child_key, "") if isinstance(parent, dict) else ""
if isinstance(value, str):
    print(value)
PY
}

detect_scope_layout() {
  local state_layout=""
  state_layout="$(json_first_string "scopeLayout" "$feature_dir/state.json" || true)"
  if [[ "$state_layout" == "per-scope-directory" ]] || [[ -f "$feature_dir/scopes/_index.md" ]]; then
    echo "per-scope-directory"
  else
    echo "single-file"
  fi
}

combined_scopes_tmp=""
scope_section_tmp_files=()

build_scope_analysis_units() {
  local scope_path="$1"
  local current_tmp=""
  local current_label=""
  local line=""

  if [[ "$scope_layout" != "single-file" ]] || [[ "$(basename "$scope_path")" != "scopes.md" ]]; then
    scope_analysis_files+=("$scope_path")
    scope_analysis_labels+=("${scope_path#$feature_dir/}")
    return
  fi

  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" =~ ^##[[:space:]]+Scope[[:space:]]+[0-9]+: ]]; then
      if [[ -n "$current_tmp" ]]; then
        scope_analysis_files+=("$current_tmp")
        scope_analysis_labels+=("$current_label")
      fi

      current_tmp="$(mktemp)"
      scope_section_tmp_files+=("$current_tmp")
      current_label="$(printf '%s' "$line" | sed -E 's/^##[[:space:]]+//')"
      printf '%s\n' "$line" > "$current_tmp"
      continue
    fi

    if [[ -n "$current_tmp" ]]; then
      if [[ "$line" =~ ^##[[:space:]]+Shared[[:space:]]+Planning[[:space:]]+Expectations ]]; then
        scope_analysis_files+=("$current_tmp")
        scope_analysis_labels+=("$current_label")
        current_tmp=""
        current_label=""
        continue
      fi

      printf '%s\n' "$line" >> "$current_tmp"
    fi
  done < "$scope_path"

  if [[ -n "$current_tmp" ]]; then
    scope_analysis_files+=("$current_tmp")
    scope_analysis_labels+=("$current_label")
  fi
}

scope_analysis_label() {
  local index="$1"
  if [[ "$index" -lt ${#scope_analysis_labels[@]} ]]; then
    printf '%s\n' "${scope_analysis_labels[$index]}"
  else
    printf '%s\n' "${scope_analysis_files[$index]#$feature_dir/}"
  fi
}

cleanup_tmp_artifacts() {
  if [[ -n "$combined_scopes_tmp" ]] && [[ -f "$combined_scopes_tmp" ]]; then
    rm -f "$combined_scopes_tmp"
  fi

  if [[ ${#scope_section_tmp_files[@]} -gt 0 ]]; then
    rm -f "${scope_section_tmp_files[@]}"
  fi
}

trap cleanup_tmp_artifacts EXIT

scope_layout="$(detect_scope_layout)"
scope_index_file="$feature_dir/scopes/_index.md"
scope_files=()
scope_analysis_files=()
scope_analysis_labels=()
report_files=()
# The following per-feature artifact paths are exported into the environment and
# consumed by child guard scripts / embedded heredocs that shellcheck cannot
# follow, so it reports them as unused (SC2034) — they are not. Keep declared.
# shellcheck disable=SC2034
scenario_manifest_file="$feature_dir/scenario-manifest.json"
# shellcheck disable=SC2034
lockdown_approvals_file="$feature_dir/lockdown-approvals.json"
# shellcheck disable=SC2034
invalidation_ledger_file="$feature_dir/invalidation-ledger.json"
# shellcheck disable=SC2034
transition_requests_file="$feature_dir/transition-requests.json"
# shellcheck disable=SC2034
rework_queue_file="$feature_dir/rework-queue.json"
framework_ownership_lint_script="$SCRIPT_DIR/agent-ownership-lint.sh"

if [[ "$scope_layout" == "per-scope-directory" ]]; then
  while IFS= read -r scope_path; do
    scope_files+=("$scope_path")
  done < <(find "$feature_dir/scopes" -mindepth 2 -maxdepth 2 -type f -name 'scope.md' | sort)

  while IFS= read -r scope_report_path; do
    report_files+=("$scope_report_path")
  done < <(find "$feature_dir/scopes" -mindepth 2 -maxdepth 2 -type f -name 'report.md' | sort)
else
  scope_files+=("$feature_dir/scopes.md")
  report_files+=("$feature_dir/report.md")
fi

for scope_path in "${scope_files[@]}"; do
  build_scope_analysis_units "$scope_path"
done

if [[ ${#scope_analysis_files[@]} -eq 0 ]]; then
  scope_analysis_files=("${scope_files[@]}")
  for scope_path in "${scope_files[@]}"; do
    scope_analysis_labels+=("${scope_path#$feature_dir/}")
  done
fi

scopes_file=""
if [[ ${#scope_files[@]} -gt 0 ]]; then
  if [[ ${#scope_files[@]} -eq 1 ]]; then
    scopes_file="${scope_files[0]}"
  else
    combined_scopes_tmp="$(mktemp)"
    for scope_path in "${scope_files[@]}"; do
      printf '%%%% %s %%%%\n' "$scope_path" >> "$combined_scopes_tmp"
      cat "$scope_path" >> "$combined_scopes_tmp"
      printf '\n' >> "$combined_scopes_tmp"
    done
    scopes_file="$combined_scopes_tmp"
  fi
fi
# shellcheck disable=SC2034  # exported/consumed by child guard scripts; not unused.
scope_file="$scopes_file"

relative_artifact_path() {
  local artifact_path="$1"
  echo "${artifact_path#$feature_dir/}"
}

count_gherkin_scenarios() {
  local total=0
  local scope_path=""
  for scope_path in "${scope_files[@]}"; do
    [[ -f "$scope_path" ]] || continue
    total=$((total + $(grep -cE '^[[:space:]]*Scenario( Outline)?:' "$scope_path" || true)))
  done
  echo "$total"
}

echo "============================================================"
echo "  BUBBLES STATE TRANSITION GUARD"
echo "  Feature: $feature_dir"
echo "  Timestamp: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
echo "============================================================"
fun_banner
fun_message guard_start
echo ""

# =============================================================================
# CHECK 1: Required artifacts exist
# =============================================================================
echo "--- Check 1: Required Artifacts ---"
required_files=("spec.md" "design.md" "uservalidation.md" "state.json")
for required_file in "${required_files[@]}"; do
  if [[ -f "$feature_dir/$required_file" ]]; then
    pass "Required artifact exists: $required_file"
  else
    fail "Missing required artifact: $feature_dir/$required_file"
  fi
done

if [[ "$scope_layout" == "per-scope-directory" ]]; then
  if [[ -f "$scope_index_file" ]]; then
    pass "Required artifact exists: scopes/_index.md"
  else
    fail "Missing required artifact: $scope_index_file"
  fi

  if [[ ${#scope_files[@]} -gt 0 ]]; then
    pass "Per-scope layout contains ${#scope_files[@]} scope file(s)"
  else
    fail "Per-scope layout requires at least one scopes/NN-name/scope.md file"
  fi

  missing_scope_reports=0
  for scope_path in "${scope_files[@]}"; do
    scope_report_path="$(dirname "$scope_path")/report.md"
    if [[ -f "$scope_report_path" ]]; then
      pass "Scope report exists: ${scope_report_path#$feature_dir/}"
    else
      fail "Missing scope report for ${scope_path#$feature_dir/}: ${scope_report_path#$feature_dir/}"
      missing_scope_reports=$((missing_scope_reports + 1))
    fi
  done

  if [[ "$missing_scope_reports" -eq 0 ]] && [[ ${#scope_files[@]} -gt 0 ]]; then
    pass "Every per-scope directory has a report.md file"
  fi
else
  if [[ -f "$feature_dir/scopes.md" ]]; then
    pass "Required artifact exists: scopes.md"
  else
    fail "Missing required artifact: $feature_dir/scopes.md"
  fi

  if [[ -f "$feature_dir/report.md" ]]; then
    pass "Required artifact exists: report.md"
  else
    fail "Missing required artifact: $feature_dir/report.md"
  fi
fi
echo ""

# =============================================================================
# CHECK 2: state.json structural integrity
# =============================================================================
echo "--- Check 2: state.json Integrity ---"
state_file="$feature_dir/state.json"
if [[ ! -f "$state_file" ]]; then
  fail "state.json does not exist"
  # Can't do remaining checks without state.json
  echo ""
  echo "RESULT: BLOCKED ($failures failures)"
  exit 1
fi

state_status="$({ grep -Eo '"status"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" | head -n 1 | sed -E 's/.*"([^"]+)"/\1/'; } || true)"
state_workflow_mode="$({ grep -Eo '"workflowMode"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" | head -n 1 | sed -E 's/.*"([^"]+)"/\1/'; } || true)"
state_plan_maturity_only="$(json_first_bool "planMaturityOnly" "$state_file" || true)"
wi_canonical_count="$({ grep -Eo '"canonicalCount"[[:space:]]*:[[:space:]]*[0-9]+' "$state_file" | head -n 1 | sed -E 's/.*:[[:space:]]*([0-9]+)/\1/'; } || true)"
wi_provisional_count="$({ grep -Eo '"provisionalIntakeCount"[[:space:]]*:[[:space:]]*[0-9]+' "$state_file" | head -n 1 | sed -E 's/.*:[[:space:]]*([0-9]+)/\1/'; } || true)"
wi_post_migration_target="$({ grep -Eo '"postMigrationTargetCount"[[:space:]]*:[[:space:]]*[0-9]+' "$state_file" | head -n 1 | sed -E 's/.*:[[:space:]]*([0-9]+)/\1/'; } || true)"
wi_migration_status="$({ grep -Eo '"migrationStatus"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" | head -n 1 | sed -E 's/.*"migrationStatus"[[:space:]]*:[[:space:]]*"([^"]+)"/\1/'; } || true)"
wi_migration_source="$({ grep -Eo '"migrationSource"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" | head -n 1 | sed -E 's/.*"migrationSource"[[:space:]]*:[[:space:]]*"([^"]+)"/\1/'; } || true)"
wi_trace_matrix="$({ grep -Eo '"traceMatrix"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" | head -n 1 | sed -E 's/.*"traceMatrix"[[:space:]]*:[[:space:]]*"([^"]+)"/\1/'; } || true)"

if [[ -z "$state_status" ]]; then
  fail "state.json missing 'status' field"
fi

if [[ -z "$state_workflow_mode" ]]; then
  fail "state.json missing 'workflowMode' field (required for status ceiling enforcement)"
fi

info "Current state.json status: ${state_status:-MISSING}"
info "Current workflowMode: ${state_workflow_mode:-MISSING}"
if [[ "$state_plan_maturity_only" == "true" ]]; then
  info "Current planMaturityOnly: true"
fi
echo ""

# =============================================================================
# CHECK 2B: workflowMode consistency (Gate G074)
# =============================================================================
# Detects contradictions between top-level workflowMode and
# policySnapshot.workflowMode. Both fields claim to describe the active mode
# but are written by different code paths; drift between them means at least
# one is fabricated.
echo "--- Check 2B: workflowMode Consistency ---"
policy_workflow_mode="$(json_nested_string "policySnapshot" "workflowMode" "$state_file" || true)"
if [[ -z "$policy_workflow_mode" ]]; then
  info "No policySnapshot.workflowMode present — skipping consistency check"
elif [[ -z "$state_workflow_mode" ]]; then
  info "Top-level workflowMode missing — skipping consistency check"
elif [[ "$state_workflow_mode" != "$policy_workflow_mode" ]]; then
  fail "workflowMode contradiction: top-level='$state_workflow_mode' vs policySnapshot='$policy_workflow_mode' — at least one was fabricated"
else
  pass "workflowMode consistent across top-level and policySnapshot ($state_workflow_mode)"
fi
echo ""

# =============================================================================
# CHECK 2A: WI parity integrity (canonical + provisional intake mode)
# =============================================================================
echo "--- Check 2A: WI Parity Integrity ---"
if [[ -n "$wi_canonical_count$wi_provisional_count$wi_post_migration_target$wi_migration_status" ]]; then
  info "Detected wiParity metadata in state.json"

  if [[ -z "$wi_canonical_count" ]] || [[ -z "$wi_provisional_count" ]] || [[ -z "$wi_post_migration_target" ]] || [[ -z "$wi_migration_status" ]]; then
    fail "wiParity metadata is incomplete (requires canonicalCount, provisionalIntakeCount, postMigrationTargetCount, migrationStatus)"
  else
    expected_wi_total=$((wi_canonical_count + wi_provisional_count))
    if [[ "$expected_wi_total" -eq "$wi_post_migration_target" ]]; then
      pass "wiParity equation valid: canonical ($wi_canonical_count) + provisional ($wi_provisional_count) = postMigrationTarget ($wi_post_migration_target)"
    else
      fail "wiParity mismatch: canonical ($wi_canonical_count) + provisional ($wi_provisional_count) != postMigrationTarget ($wi_post_migration_target)"
    fi

    case "$wi_migration_status" in
      proposed_not_activated|activated|not_applicable)
        pass "wiParity migrationStatus is valid: $wi_migration_status"
        ;;
      *)
        fail "wiParity migrationStatus '$wi_migration_status' is invalid (allowed: proposed_not_activated, activated, not_applicable)"
        ;;
    esac

    if [[ "$wi_migration_status" == "proposed_not_activated" ]] && [[ "$wi_provisional_count" -gt 0 ]]; then
      pass "Dual-count mode recognized (canonical + provisional tracked separately)"
    fi

    if [[ "$wi_migration_status" == "activated" ]] && [[ "$wi_provisional_count" -gt 0 ]]; then
      fail "migrationStatus 'activated' requires provisionalIntakeCount=0 (found $wi_provisional_count)"
    fi
  fi

  if [[ -n "$wi_migration_source" ]]; then
    wi_migration_source_file="${wi_migration_source%%#*}"
    if [[ -f "$feature_dir/$wi_migration_source_file" ]]; then
      pass "wiParity migrationSource file exists: $wi_migration_source_file"
    else
      fail "wiParity migrationSource file missing: $feature_dir/$wi_migration_source_file"
    fi
  fi

  if [[ -n "$wi_trace_matrix" ]]; then
    if [[ -f "$feature_dir/$wi_trace_matrix" ]]; then
      pass "wiParity traceMatrix file exists: $wi_trace_matrix"
    else
      fail "wiParity traceMatrix file missing: $feature_dir/$wi_trace_matrix"
    fi
  fi
else
  info "No wiParity metadata found (dual-count checks skipped)"
fi
echo ""

# =============================================================================
# CHECK 3: Status ceiling enforcement
# =============================================================================
echo "--- Check 3: Status Ceiling Enforcement ---"
if [[ -n "$state_workflow_mode" ]]; then
  state_status_ceiling="$(resolve_workflow_status_ceiling "$state_workflow_mode" || true)"
  if [[ -z "$state_status_ceiling" ]]; then
    fail "Unknown workflow mode '$state_workflow_mode' — cannot verify status ceiling from workflows.yaml"
  elif [[ "$state_status" == "$state_status_ceiling" ]]; then
    pass "Workflow mode '$state_workflow_mode' permits current status '$state_status' (ceiling: $state_status_ceiling)"
  elif [[ "$state_status" == "done" && "$state_status_ceiling" != "done" ]]; then
    fail "Workflow mode '$state_workflow_mode' ceiling is '$state_status_ceiling', NOT 'done'"
  elif [[ "$state_status_ceiling" == "done" ]]; then
    info "Workflow mode '$state_workflow_mode' allows status 'done'; current status is '$state_status'"
  else
    info "Workflow mode '$state_workflow_mode' ceiling is '$state_status_ceiling'; current status is '$state_status'"
  fi
fi

if [[ "$state_plan_maturity_only" == "true" && "$state_status" == "done" ]]; then
  fail "state.json planMaturityOnly=true is incompatible with status 'done' — planning maturity must stop at the workflow status ceiling"
elif [[ "$state_plan_maturity_only" == "true" ]]; then
  pass "state.json planMaturityOnly=true is not claiming delivery-done status"
fi
echo ""

# =============================================================================
# CHECK 3B: Source code edit lockout for planning-only modes (Gate G073)
# =============================================================================
echo "--- Check 3B: Source Code Edit Lockout (Gate G073) ---"

# Determine if the current mode forbids source code edits
ceiling_forbids_code="false"
ceiling_label="$(resolve_workflow_status_ceiling "$state_workflow_mode" || true)"
if [[ -n "$ceiling_label" && "$ceiling_label" != "done" ]]; then
  ceiling_forbids_code="true"
fi

if [[ "$ceiling_forbids_code" == "true" ]]; then
  git_repo_root=""
  if command -v git &>/dev/null && git -C "$feature_dir" rev-parse --is-inside-work-tree &>/dev/null 2>&1; then
    git_repo_root="$(git -C "$feature_dir" rev-parse --show-toplevel 2>/dev/null || true)"
  fi

  # Check if git is available and the target feature lives inside a repo.
  if [[ -n "$git_repo_root" ]]; then
    # Get source code files modified in the working tree + staged + last commit
    # relative to the repo root, then filter for implementation file extensions
    source_code_violations=0
    source_code_pattern='\.(go|rs|py|ts|tsx|js|jsx|sql|proto|yaml|yml|toml|json|css|scss|html)$'
    # Exclude specs/ docs/ .github/ .specify/ paths — those are allowed
    allowed_path_pattern='^(specs/|docs/|\.github/|\.specify/|CHANGELOG|README|LICENSE|VERSION)'

    # ── v4.1.0: Deliverable Files Manifest (Gate G073 refinement) ─────────
    # When state.json declares `deliverableFiles[]`, those files are
    # permitted edits even under restrictive ceilings (e.g.
    # `delivered_pending_activation`, `specs_hardened`, `validated`,
    # `docs_updated`). This is the honest replacement for the v4.0.x
    # blanket lockout, which was a false positive for adapter-readiness,
    # dark-launch, and migration-pending-cutover modes.
    #
    # Manifest entries may be:
    #   - exact file path: "<product>/home-lab/apply.sh"
    #   - directory prefix (trailing '/'): "<product>/home-lab/"
    #   - recursive glob (trailing '/**'): "<product>/home-lab/tests/**"
    deliverable_files_list=""
    if command -v python3 &>/dev/null; then
      deliverable_files_list="$(python3 -c "
import json
try:
    d=json.load(open('$state_file'))
    for f in (d.get('deliverableFiles') or []):
        if isinstance(f,str) and f.strip():
            print(f.strip())
except Exception:
    pass" 2>/dev/null || true)"
    fi

    is_deliverable_file() {
      local f="$1"
      [[ -z "$deliverable_files_list" ]] && return 1
      local df
      while IFS= read -r df; do
        [[ -z "$df" ]] && continue
        if [[ "$f" == "$df" ]]; then return 0; fi
        # Recursive glob: "<prefix>/**"
        if [[ "$df" == */\*\* && "$f" == "${df%/\*\*}/"* ]]; then return 0; fi
        # Directory prefix: "<prefix>/"
        if [[ "$df" == */ && "$f" == "$df"* ]]; then return 0; fi
      done <<< "$deliverable_files_list"
      return 1
    }

    if [[ -n "$deliverable_files_list" ]]; then
      manifest_count=$(printf '%s\n' "$deliverable_files_list" | grep -c .)
      info "deliverableFiles[] manifest present ($manifest_count entries) — declared files permitted under ceiling '$ceiling_label'"
    fi

    # Check staged files
    while IFS= read -r changed_file; do
      [[ -z "$changed_file" ]] && continue
      if echo "$changed_file" | grep -qE "$source_code_pattern"; then
        if ! echo "$changed_file" | grep -qE "$allowed_path_pattern"; then
          if is_deliverable_file "$changed_file"; then
            pass "Staged file '$changed_file' is declared in deliverableFiles[] manifest — permitted under ceiling '$ceiling_label'"
            continue
          fi
          fail "Mode '$state_workflow_mode' (ceiling: $ceiling_label) forbids source code edits, but staged file modified: $changed_file (add to deliverableFiles[] in state.json if intentional)"
          source_code_violations=$((source_code_violations + 1))
        fi
      fi
    done < <(git -C "$git_repo_root" diff --cached --name-only 2>/dev/null || true)

    # Check unstaged working tree changes
    while IFS= read -r changed_file; do
      [[ -z "$changed_file" ]] && continue
      if echo "$changed_file" | grep -qE "$source_code_pattern"; then
        if ! echo "$changed_file" | grep -qE "$allowed_path_pattern"; then
          if is_deliverable_file "$changed_file"; then
            pass "Working-tree file '$changed_file' is declared in deliverableFiles[] manifest — permitted under ceiling '$ceiling_label'"
            continue
          fi
          fail "Mode '$state_workflow_mode' (ceiling: $ceiling_label) forbids source code edits, but working tree file modified: $changed_file (add to deliverableFiles[] in state.json if intentional)"
          source_code_violations=$((source_code_violations + 1))
        fi
      fi
    done < <(git -C "$git_repo_root" diff --name-only 2>/dev/null || true)

    # Check the most recent commit (if it exists and was made during this workflow)
    last_commit_msg="$(git -C "$git_repo_root" log -1 --format='%s' 2>/dev/null || true)"
    if [[ -n "$last_commit_msg" ]]; then
      while IFS= read -r changed_file; do
        [[ -z "$changed_file" ]] && continue
        if echo "$changed_file" | grep -qE "$source_code_pattern"; then
          if ! echo "$changed_file" | grep -qE "$allowed_path_pattern"; then
            if is_deliverable_file "$changed_file"; then
              continue
            fi
            warn "Mode '$state_workflow_mode' (ceiling: $ceiling_label) forbids source code edits — last commit touched: $changed_file (review commit: $last_commit_msg)"
          fi
        fi
      done < <(git -C "$git_repo_root" diff --name-only HEAD~1 HEAD -- 2>/dev/null || true)
    fi

    if [[ "$source_code_violations" -eq 0 ]]; then
      pass "No undeclared source code edits detected under mode '$state_workflow_mode' (ceiling: $ceiling_label)"
    else
      fail "Found $source_code_violations source code file(s) modified under mode '$state_workflow_mode' that are NOT declared in deliverableFiles[] — declare them in state.json or use a delivery mode (ceiling: $ceiling_label)"
    fi
  else
    info "Git not available or target feature is not in a repo — skipping source code edit lockout check"
  fi
else
  pass "Workflow mode '$state_workflow_mode' permits source code edits (ceiling allows implementation)"
fi
echo ""

# =============================================================================
# CHECKS 3A, 3H, 3C, 3D, 3E, 3F: v3 control-plane gates — policy provenance
# (G055), validate certification (G056), scenario manifest (G057),
# lockdown/regression contracts (G058/G059), scenario-first TDD (G060), and
# transition/rework packet closure (G061/G063). Extracted to a guards/ fragment
# (M4 split) and sourced in this shell scope (byte-identical). Check 3G stays
# inline because it carries the BUG-001 timeout wrapper.
# =============================================================================
source "$SCRIPT_DIR/guards/control-plane-checks.sh"

# =============================================================================
# CHECK 3G: Framework ownership/result contract integrity (G042/G063/G064)
# =============================================================================
echo "--- Check 3G: Framework Ownership And Result Contract (G042/G063/G064) ---"
if [[ -x "$framework_ownership_lint_script" || -f "$framework_ownership_lint_script" ]]; then
  _c3g_start=$(date +%s)
  _c3g_rc=0
  bubbles_run_with_timeout 30 bash "$framework_ownership_lint_script" >/tmp/bubbles-agent-ownership-lint.$$ 2>&1 || _c3g_rc=$?
  _c3g_elapsed=$(( $(date +%s) - _c3g_start ))
  if [[ "$_c3g_rc" -eq 124 ]]; then
    fail "Framework ownership lint TIMED OUT after 30s (BUG-001 guard) — G042/G063/G064 not certified. Inspect $framework_ownership_lint_script for an unbounded walk."
  elif [[ "$_c3g_rc" -eq 0 ]]; then
    pass "Framework ownership lint passed — artifact ownership enforcement, concrete result contract, and child workflow policy are internally consistent (${_c3g_elapsed}s)"
  else
    fail "Framework ownership lint failed — G042/G063/G064 cannot be certified during state transition"
    while IFS= read -r lint_line; do
      [[ -n "$lint_line" ]] || continue
      echo "   → $lint_line"
    done < /tmp/bubbles-agent-ownership-lint.$$
  fi
  if (( _c3g_elapsed > 30 )); then
    warn "Check 3G wall-clock ${_c3g_elapsed}s exceeded the 30s budget"
  fi
  rm -f /tmp/bubbles-agent-ownership-lint.$$
else
  fail "Framework ownership lint script not found at $framework_ownership_lint_script — cannot enforce G042/G063/G064"
fi
echo ""

# =============================================================================
# CHECK 4: ALL DoD items must be checked [x] — ZERO unchecked allowed
# =============================================================================
echo "--- Check 4: DoD Completion (Zero Unchecked) ---"
total_checked=0
total_unchecked=0
for scope_path in "${scope_files[@]}"; do
  [[ -f "$scope_path" ]] || continue
  total_checked=$((total_checked + $(grep -cE '^\- \[x\] ' "$scope_path" || true)))
  total_unchecked=$((total_unchecked + $(grep -cE '^\- \[ \] ' "$scope_path" || true)))
done
total_dod=$((total_checked + total_unchecked))

info "DoD items total: $total_dod (checked: $total_checked, unchecked: $total_unchecked)"

if [[ "$total_dod" -eq 0 ]]; then
  fail "Resolved scope artifacts have ZERO DoD checkbox items — cannot verify completion"
elif [[ "$total_unchecked" -gt 0 ]]; then
  fail "Resolved scope artifacts have $total_unchecked UNCHECKED DoD items — ALL must be [x] for 'done'"
  shown_unchecked=0
  for scope_path in "${scope_files[@]}"; do
    [[ -f "$scope_path" ]] || continue
    while IFS= read -r unchecked_line; do
      [[ -n "$unchecked_line" ]] || continue
      echo "   → ${scope_path#$feature_dir/}: $unchecked_line"
      shown_unchecked=$((shown_unchecked + 1))
      if [[ "$shown_unchecked" -ge 10 ]]; then
        break 2
      fi
    done < <(grep -E '^\- \[ \] ' "$scope_path" || true)
  done
else
  pass "All $total_checked DoD items are checked [x]"
fi
echo ""

# =============================================================================
# CHECK 4A: DoD format manipulation detection (Gate G041)
# =============================================================================
# Detects agents that bypass Check 4 by reformatting DoD checkboxes into
# non-checkbox formats (e.g., "- (deferred) Item", "- ~~Item~~", "- *Item*",
# "- Item" without checkbox). Only `- [ ] ` and `- [x] ` are valid DoD
# item formats. Any other `- ` prefixed items inside a "Definition of Done"
# section are format manipulation.
# =============================================================================
echo "--- Check 4A: DoD Format Manipulation Detection (Gate G041) ---"
total_manipulated=0
# BUG-005: precompiled patterns (bash builtins replace per-line echo|grep forks).
# `_c4a_dod_header_re` is matched case-INSENSITIVELY (original grep -qiE); the
# rest are case-SENSITIVE (original grep -qE).
_c4a_dod_header_re='^#{1,4}.*Definition of Done|^#{1,4}.*DoD'
_c4a_heading_re='^#{1,4} '
_c4a_listitem_re='^\- '
_c4a_checkbox_re='^\- \[(x| )\] '
for scope_path in "${scope_files[@]}"; do
  [[ -f "$scope_path" ]] || continue

  # Extract lines inside DoD sections and check for non-checkbox list items
  in_dod=0
  line_num=0
  while IFS= read -r line; do
    line_num=$((line_num + 1))

    # Detect DoD section headers (case-INSENSITIVE — matches the original
    # grep -qiE). BUG-005: bash builtin instead of an echo|grep fork per line.
    _c4a_is_dod_header=0
    shopt -s nocasematch
    [[ "$line" =~ $_c4a_dod_header_re ]] && _c4a_is_dod_header=1
    shopt -u nocasematch
    if [[ "$_c4a_is_dod_header" -eq 1 ]]; then
      in_dod=1
      continue
    fi

    # Exit DoD section on next heading or scope boundary (case-sensitive)
    if [[ "$in_dod" -eq 1 ]] && [[ "$line" =~ $_c4a_heading_re ]]; then
      in_dod=0
      continue
    fi

    # While inside DoD section, check list items
    if [[ "$in_dod" -eq 1 ]]; then
      # Valid formats: "- [ ] " or "- [x] "
      # Invalid: "- (deferred)", "- ~~text~~", "- *text*", "- text" (no checkbox)
      if [[ "$line" =~ $_c4a_listitem_re ]] && ! [[ "$line" =~ $_c4a_checkbox_re ]]; then
        fail "DoD format manipulation detected in ${scope_path#$feature_dir/} line $line_num: ${line:0:100}"
        fun_message format_bypass
        total_manipulated=$((total_manipulated + 1))
      fi
    fi
  done < "$scope_path"
done

if [[ "$total_manipulated" -gt 0 ]]; then
  fail "$total_manipulated DoD item(s) have been reformatted to bypass checkbox validation — MANIPULATION DETECTED (Gate G041)"
  fun_message manipulation_detected
  info "Valid DoD format is ONLY: '- [ ] Description' or '- [x] Description'"
  info "Patterns like '- (deferred) ...', '- ~~...~~', '- Item without checkbox' are FORBIDDEN"
else
  pass "No DoD format manipulation detected — all DoD items use checkbox format"
fi
echo ""

# =============================================================================
# CHECK 4B: Non-canonical scope status detection (Gate G041)
# =============================================================================
# Only four scope statuses are valid: "Not Started", "In Progress", "Done",
# "Blocked". Any other status string (e.g., "Deferred", "Deferred — Planned
# Improvement", "Skipped", "N/A") is an invented status used to bypass the
# guard's scope status checks.
# =============================================================================
echo "--- Check 4B: Scope Status Canonicality (Gate G041) ---"
non_canonical_statuses=0
for scope_path in "${scope_files[@]}"; do
  [[ -f "$scope_path" ]] || continue

  # Find all **Status:** lines. Blockquote (>-prefixed) lines are header/summary
  # prose (e.g. `> **Status:** all scopes Not Started (planning refreshed …)`),
  # NOT canonical per-scope status declarations — exclude them so a rollup
  # summary is never mis-read as an invented status value (BUG-006).
  while IFS= read -r status_line; do
    [[ -n "$status_line" ]] || continue
    # Extract the status value after "**Status:**"
    status_value="$(echo "$status_line" | sed -E 's/.*\*\*Status:\*\*[[:space:]]*//' | sed -E 's/[[:space:]]*$//')"

    # v4.1.0: tolerate canonical-status followed by parenthesized annotation,
    # e.g. "Done (completed_owned)", "Done (lockdown-deferred-FR-020)",
    # "Blocked (awaiting-operator-commit)". The base status before the
    # parenthesis is still required to be canonical; the annotation is
    # informational (typically routing context from the owning agent).
    base_status="$(echo "$status_value" | sed -E 's/[[:space:]]*\(.*\)[[:space:]]*$//' | sed -E 's/[[:space:]]+$//')"

    # Check against canonical values
    case "$base_status" in
      "Not Started"|"In Progress"|"Done"|"Blocked")
        # Valid canonical status (with or without parenthesized annotation)
        ;;
      *)
        fail "Non-canonical scope status detected in ${scope_path#$feature_dir/}: '$status_value' — ONLY 'Not Started', 'In Progress', 'Done', 'Blocked' (optionally followed by '(<annotation>)') are valid"
        fun_message invented_status
        non_canonical_statuses=$((non_canonical_statuses + 1))
        ;;
    esac
  done < <(bubbles_status_lines "$scope_path")
done

if [[ "$non_canonical_statuses" -gt 0 ]]; then
  fail "$non_canonical_statuses scope(s) have invented/non-canonical status values — MANIPULATION DETECTED (Gate G041)"
  info "Canonical scope statuses are ONLY: 'Not Started', 'In Progress', 'Done', 'Blocked'"
  info "Invented statuses like 'Deferred', 'Skipped', 'N/A', 'Deferred — Planned Improvement' are FORBIDDEN"
  info "Parenthesized annotations such as 'Done (completed_owned)' or 'Blocked (awaiting-operator-commit)' are permitted"
else
  pass "All scope statuses are canonical (Not Started / In Progress / Done / Blocked, optionally with annotation)"
fi
echo ""

# =============================================================================
# CHECK 5: Scope status cross-reference — scopes marked "Done" in scopes.md
# must match state.json completedScopes
# =============================================================================
echo "--- Check 5: Scope Status Cross-Reference ---"
not_started_scopes=0
in_progress_scopes=0
blocked_scopes=0
done_scopes=0
for scope_path in "${scope_files[@]}"; do
  [[ -f "$scope_path" ]] || continue
  # Count per-scope statuses over the canonical status lines only (blockquote
  # summary lines excluded via the shared helper — BUG-006 / IMP-009).
  _scope_status_lines="$(bubbles_status_lines "$scope_path")"
  not_started_scopes=$((not_started_scopes + $(printf '%s' "$_scope_status_lines" | grep -cE '\*\*Status:\*\*.*Not Started' || true)))
  in_progress_scopes=$((in_progress_scopes + $(printf '%s' "$_scope_status_lines" | grep -cE '\*\*Status:\*\*.*In Progress' || true)))
  blocked_scopes=$((blocked_scopes + $(printf '%s' "$_scope_status_lines" | grep -cE '\*\*Status:\*\*.*Blocked' || true)))
  done_scopes=$((done_scopes + $(printf '%s' "$_scope_status_lines" | grep -cE '\*\*Status:\*\*.*Done' || true)))
done
total_scopes=$((not_started_scopes + in_progress_scopes + blocked_scopes + done_scopes))

info "Resolved scopes: total=$total_scopes, Done=$done_scopes, In Progress=$in_progress_scopes, Not Started=$not_started_scopes, Blocked=$blocked_scopes"

if [[ "$total_scopes" -eq 0 ]]; then
  fail "Resolved scope artifacts have no scope status markers"
elif [[ "$not_started_scopes" -gt 0 ]]; then
  fail "Resolved scope artifacts have $not_started_scopes scope(s) still marked 'Not Started' — ALL scopes must be Done"
elif [[ "$in_progress_scopes" -gt 0 ]]; then
  fail "Resolved scope artifacts have $in_progress_scopes scope(s) still marked 'In Progress' — ALL scopes must be Done"
elif [[ "$blocked_scopes" -gt 0 ]]; then
  fail "Resolved scope artifacts have $blocked_scopes scope(s) still marked 'Blocked' — ALL scopes must be Done"
else
  pass "All $done_scopes scope(s) are marked Done"
fi

state_completed_scopes_count="$({
  certification_scopes_block="$({
    grep -A40 '"certification"' "$state_file" 2>/dev/null \
      | awk '/"completedScopes"[[:space:]]*:/ {capture=1} capture {print} capture && /\]/ {exit}'
  } || true)"

  if [[ -n "$certification_scopes_block" ]]; then
    echo "$certification_scopes_block" \
      | sed -E '1s/.*"completedScopes"[[:space:]]*:[[:space:]]*\[//' \
      | grep -cE '"[^"]+"' || true
  else
    awk '/"completedScopes"[[:space:]]*:/ {capture=1} capture {print} capture && /\]/ {exit}' "$state_file" \
      | sed -E '1s/.*"completedScopes"[[:space:]]*:[[:space:]]*\[//' \
      | grep -cE '"[^"]+"' || true
  fi
} || true)"

if [[ "$done_scopes" -gt 0 ]] && [[ "$state_completed_scopes_count" -eq 0 ]]; then
  fail "Resolved scope artifacts report $done_scopes Done scope(s) but state.json completedScopes is EMPTY — state.json integrity failure"
elif [[ "$done_scopes" -ne "$state_completed_scopes_count" ]]; then
  fail "completedScopes count ($state_completed_scopes_count) does not match artifact Done scope count ($done_scopes) — state.json integrity failure"
else
  pass "completedScopes count matches artifact Done scope count ($done_scopes)"
fi
echo ""

# =============================================================================
# CHECK 5B: _index.md ↔ scope.md status parity (Gate G075)
# =============================================================================
# In per-scope-directory layout, the _index.md "Status" column is a separate
# source of truth from each scope-local scope.md. If they disagree, at least
# one is fabricated. The 042 fabrication left _index.md showing every scope
# as "In Progress" while individual scope.md files claimed "Done".
echo "--- Check 5B: _index.md ↔ scope.md Status Parity ---"
if [[ "$scope_layout" == "per-scope-directory" ]] && [[ -f "$scope_index_file" ]]; then
  index_parity_failures=0
  index_parity_checked=0
  # Each scope.md path looks like: .../scopes/NN-name/scope.md
  for scope_path in "${scope_files[@]}"; do
    [[ -f "$scope_path" ]] || continue
    scope_dir_name="$(basename "$(dirname "$scope_path")")"
    # Strip leading "NN-" prefix to get the scope's natural-language identifier
    scope_dir_suffix="${scope_dir_name#[0-9]*-}"
    scope_dir_num="${scope_dir_name%%-*}"
    scope_status_local="$(grep -m1 -E '^\*\*Status:\*\*' "$scope_path" \
      | sed -E 's/.*\*\*Status:\*\*[[:space:]]*([A-Za-z ]+).*/\1/' \
      | sed -E 's/[[:space:]]+$//' || true)"
    if [[ -z "$scope_status_local" ]]; then
      continue
    fi

    # Find the row in _index.md that begins with the scope number (allowing
    # leading zeros, optional leading pipe and whitespace).
    index_row="$(grep -E "^\|[[:space:]]*0*${scope_dir_num#0}[[:space:]]*\|" "$scope_index_file" \
      | head -n 1 || true)"
    if [[ -z "$index_row" ]]; then
      # Fall back to matching by directory suffix in the row text
      index_row="$(grep -F "$scope_dir_suffix" "$scope_index_file" \
        | grep -E '^\|' | head -n 1 || true)"
    fi
    if [[ -z "$index_row" ]]; then
      warn "_index.md has no row matching scope $scope_dir_name — cannot verify parity"
      continue
    fi
    # Last pipe-delimited cell is the Status column
    index_status="$(echo "$index_row" \
      | awk -F'|' '{ for (i=NF; i>=1; i--) { gsub(/^[[:space:]]+|[[:space:]]+$/, "", $i); if ($i != "") { print $i; exit } } }')"
    if [[ -z "$index_status" ]]; then
      continue
    fi
    index_parity_checked=$((index_parity_checked + 1))
    if [[ "$index_status" != "$scope_status_local" ]]; then
      fail "_index.md says '$index_status' for scope $scope_dir_name but scope.md says '$scope_status_local' — fabrication indicator"
      index_parity_failures=$((index_parity_failures + 1))
    fi
  done
  if [[ "$index_parity_checked" -gt 0 ]] && [[ "$index_parity_failures" -eq 0 ]]; then
    pass "_index.md statuses match scope.md statuses for all $index_parity_checked checked scope(s)"
  elif [[ "$index_parity_checked" -eq 0 ]]; then
    info "Could not match any scope.md to an _index.md row (no rows checked)"
  fi
else
  info "_index.md parity check skipped (single-file layout or no _index.md)"
fi
echo ""

# =============================================================================
# CHECK 5C: Phantom scope detection (Gate G076)
# =============================================================================
# Every entry in completedScopes (and certification.completedScopes) MUST map
# to a real scope artifact on disk. The 042 fabrication added
# "scope-15-stochastic-sweep-remediation" to completedScopes with no
# corresponding directory or scope.md.
#
# Per-scope-directory layout only: in single-file layout, completedScopes
# entries are agent-chosen labels with no canonical mapping to scope identity,
# so we can only verify counts (Check 5 already does this).
echo "--- Check 5C: Phantom Scope Detection ---"
phantom_count=0
if [[ "$scope_layout" != "per-scope-directory" ]]; then
  info "Phantom scope detection skipped (single-file layout — entries are free-form labels)"
elif [[ -f "$state_file" ]]; then
  while IFS= read -r entry; do
    [[ -n "$entry" ]] || continue
    found=0
    # Match completedScopes entry against any scope directory by suffix
    for scope_path in "${scope_files[@]}"; do
      scope_dir_name="$(basename "$(dirname "$scope_path")")"
      scope_dir_num="${scope_dir_name%%-*}"
      # Accept either full directory name match or numeric-prefix match
      # (the entry typically looks like "scope-7-foo-bar" or "07-foo-bar").
      if [[ "$entry" == *"$scope_dir_name"* ]] \
        || [[ "$entry" == *"-${scope_dir_num#0}-"* ]] \
        || [[ "$entry" == *"-${scope_dir_num}-"* ]] \
        || [[ "$entry" == "${scope_dir_num#0}-"* ]] \
        || [[ "$entry" == "${scope_dir_num}-"* ]]; then
        found=1
        break
      fi
    done
    if [[ "$found" -eq 0 ]]; then
      fail "Phantom scope in completedScopes: '$entry' has no corresponding artifact on disk"
      phantom_count=$((phantom_count + 1))
    fi
  done < <(python3 - "$state_file" <<'PY'
import json
import sys

try:
    with open(sys.argv[1], encoding="utf-8") as fh:
        data = json.load(fh)
except Exception:
    sys.exit(0)

seen = set()
for source in (data.get("completedScopes", []),
               data.get("certification", {}).get("completedScopes", []) if isinstance(data.get("certification"), dict) else []):
    if isinstance(source, list):
        for entry in source:
            if isinstance(entry, str) and entry not in seen:
                seen.add(entry)
                print(entry)
PY
)
fi

if [[ "$phantom_count" -eq 0 ]]; then
  pass "All completedScopes entries map to real scope artifacts (or check skipped for single-file layout)"
fi
echo ""

# =============================================================================
# CHECK 5A: Stress coverage for SLA-scoped work (Gate G026)
# =============================================================================
echo "--- Check 5A: SLA Stress Coverage ---"
sla_scope_count=0
for scope_path in "${scope_files[@]}"; do
  [[ -f "$scope_path" ]] || continue

  if grep -Eiq 'latency|throughput|p95|p99|response time|sla|slo' "$scope_path"; then
    sla_scope_count=$((sla_scope_count + 1))
    if grep -Eq '^\|[[:space:]]*Stress[[:space:]]*\|' "$scope_path" || grep -Eiq 'stress' "$scope_path"; then
      pass "SLA-sensitive scope includes stress coverage: ${scope_path#$feature_dir/}"
    else
      fail "SLA-sensitive scope is missing explicit stress coverage: ${scope_path#$feature_dir/}"
    fi
  fi
done

if [[ "$sla_scope_count" -eq 0 ]]; then
  info "No SLA-sensitive scopes detected for Gate G026"
fi
echo ""

# =============================================================================
# CHECK 6: completedPhases vs required specialists
# =============================================================================
echo "--- Check 6: Specialist Phase Completion ---"
state_completed_phases_block="$({
  python3 - "$state_file" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    data = json.load(handle)

# None-safe accessors: state.json may contain explicit null values for any of
# these keys; default-arg of dict.get(...) does NOT replace None, so chain
# .get() with `or {}` / `or []` to guarantee a non-None object.
certification = (data.get("certification") or {})
execution = (data.get("execution") or {})

certification_phases = certification.get("certifiedCompletedPhases") or []
execution_phase_claims = execution.get("completedPhaseClaims") or []
legacy_phases = data.get("completedPhases") or []

if not isinstance(certification_phases, list):
    certification_phases = []
if not isinstance(execution_phase_claims, list):
    execution_phase_claims = []
if not isinstance(legacy_phases, list):
    legacy_phases = []

selected_phases = certification_phases or execution_phase_claims or legacy_phases

# v4.1.0: phaseStubs[] — a phase can be honestly declared as no-work-needed
# via state.json.execution.phaseStubs[<phase>] = {reason: "...", justification: "..."}
# or state.json.phaseStubs[<phase>]. A stubbed phase satisfies G022 IFF the
# stub entry carries a non-empty `reason` field, preventing empty-stub
# fabrication.
phase_stubs = execution.get("phaseStubs")
if not isinstance(phase_stubs, dict):
    phase_stubs = data.get("phaseStubs")
if not isinstance(phase_stubs, dict):
    phase_stubs = {}

stubbed_phases = []
for phase_name, stub_entry in phase_stubs.items():
    if not isinstance(phase_name, str):
        continue
    if isinstance(stub_entry, dict):
        reason = (stub_entry.get("reason") or "").strip() if isinstance(stub_entry.get("reason"), str) else ""
        if reason:
            stubbed_phases.append(phase_name)
    elif isinstance(stub_entry, str) and stub_entry.strip():
        stubbed_phases.append(phase_name)

# Normalize selected_phases to phase-name STRINGS before the dict.fromkeys
# dedup. Entries may be bare strings (certifiedCompletedPhases / legacy
# completedPhases) OR dict records from execution.completedPhaseClaims such as
# {"phase": "implement", "agent": "bubbles.implement"}. A dict cannot be hashed
# as a dict.fromkeys key (TypeError: unhashable type: 'dict'), which previously
# crashed Check 6 whenever certifiedCompletedPhases was empty and the fallback
# selected the dict-shaped claim list — reading ALL required phases as missing
# and emitting a false G022 failure. Map each entry to its phase-name string:
# a str stays itself; a dict yields its `phase` (else `name`) value when that
# value is itself a str; anything else is skipped.
def _phase_name(entry):
    if isinstance(entry, str):
        return entry
    if isinstance(entry, dict):
        candidate = entry.get("phase")
        if isinstance(candidate, str):
            return candidate
        candidate = entry.get("name")
        if isinstance(candidate, str):
            return candidate
    return None

normalized_selected = []
for entry in selected_phases:
    resolved = _phase_name(entry)
    if resolved is not None:
        normalized_selected.append(resolved)

# Merge: a phase satisfies G022 if it appears in either set.
merged_phases = list(dict.fromkeys(normalized_selected + stubbed_phases))
for phase in merged_phases:
    if isinstance(phase, str):
        print(f'"{phase}"')
PY
} || true)"

if [[ -n "$state_workflow_mode" ]]; then
  required_specialists=()
  case "$state_workflow_mode" in
    value-first-e2e-batch)
      required_specialists=("implement" "test" "regression" "simplify" "stabilize" "security" "docs" "validate" "audit" "chaos")
      ;;
    full-delivery)
      required_specialists=("implement" "test" "regression" "simplify" "gaps" "harden" "stabilize" "security" "validate" "audit" "chaos" "docs")
      ;;
    feature-bootstrap)
      required_specialists=("implement" "test" "regression" "simplify" "stabilize" "security" "docs" "validate" "audit")
      ;;
    bugfix-fastlane)
      required_specialists=("implement" "test" "regression" "simplify" "stabilize" "security" "validate" "audit")
      ;;
    chaos-hardening)
      required_specialists=("chaos" "implement" "test" "regression" "simplify" "stabilize" "security" "validate" "audit" "docs")
      ;;
    harden-to-doc)
      required_specialists=("harden" "implement" "test" "regression" "simplify" "stabilize" "security" "chaos" "validate" "audit" "docs")
      ;;
    gaps-to-doc)
      required_specialists=("gaps" "implement" "test" "regression" "simplify" "stabilize" "security" "chaos" "validate" "audit" "docs")
      ;;
    harden-gaps-to-doc)
      required_specialists=("harden" "gaps" "implement" "test" "regression" "simplify" "stabilize" "security" "chaos" "validate" "audit" "docs")
      ;;
    reconcile-to-doc)
      required_specialists=("implement" "test" "regression" "simplify" "stabilize" "security" "validate" "audit" "chaos" "docs")
      ;;
    test-to-doc)
      required_specialists=("test" "validate" "audit" "docs")
      ;;
    chaos-to-doc)
      required_specialists=("chaos" "validate" "audit" "docs")
      ;;
    batch-implement)
      required_specialists=("implement" "test" "regression" "simplify" "stabilize" "security" "docs" "validate" "audit" "chaos")
      ;;
    batch-harden)
      required_specialists=("harden" "implement" "test" "regression" "simplify" "stabilize" "security" "validate" "audit" "chaos" "docs")
      ;;
    batch-gaps)
      required_specialists=("gaps" "implement" "test" "regression" "simplify" "stabilize" "security" "validate" "audit" "chaos" "docs")
      ;;
    batch-harden-gaps)
      required_specialists=("harden" "gaps" "implement" "test" "regression" "simplify" "stabilize" "security" "validate" "audit" "chaos" "docs")
      ;;
    batch-improve-existing)
      required_specialists=("harden" "gaps" "implement" "test" "regression" "simplify" "stabilize" "security" "validate" "audit" "chaos" "docs")
      ;;
    batch-reconcile-to-doc)
      required_specialists=("implement" "test" "validate" "audit" "chaos" "docs")
      ;;
    product-to-delivery)
      required_specialists=("implement" "test" "regression" "simplify" "stabilize" "security" "docs" "validate" "audit" "chaos")
      ;;
    improve-existing)
      required_specialists=("harden" "gaps" "implement" "test" "regression" "simplify" "stabilize" "security" "validate" "audit" "chaos" "docs")
      ;;
    redesign-existing)
      required_specialists=("implement" "test" "regression" "simplify" "stabilize" "security" "docs" "validate" "audit" "chaos")
      ;;
    stabilize-to-doc)
      required_specialists=("stabilize" "implement" "test" "regression" "simplify" "security" "chaos" "validate" "audit" "docs")
      ;;
    security-to-doc)
      required_specialists=("security" "implement" "test" "regression" "simplify" "stabilize" "devops" "chaos" "validate" "audit" "docs")
      ;;
    regression-to-doc)
      required_specialists=("regression" "implement" "test" "simplify" "stabilize" "devops" "security" "chaos" "validate" "audit" "docs")
      ;;
    simplify-to-doc)
      required_specialists=("simplify" "test" "validate" "audit" "docs")
      ;;
    iterate)
      required_specialists=("validate" "audit")
      ;;
    stochastic-quality-sweep)
      required_specialists=("validate" "audit")
      ;;
    product-discovery)
      required_specialists=("harden" "docs" "validate" "audit")
      ;;
    validate-to-doc)
      required_specialists=("validate" "audit" "docs")
      ;;
  esac

  if [[ ${#required_specialists[@]} -gt 0 ]]; then
    missing_phases=0
    for specialist_phase in "${required_specialists[@]}"; do
      if echo "$state_completed_phases_block" | grep -qE "\"$specialist_phase\""; then
        pass "Required phase '$specialist_phase' recorded in execution/certification phase records"
      else
        fail "Required phase '$specialist_phase' NOT in execution/certification phase records (Gate G022 violation)"
        missing_phases=$((missing_phases + 1))
      fi
    done
    if [[ "$missing_phases" -gt 0 ]]; then
      fail "$missing_phases specialist phase(s) missing — work was NOT executed through the full pipeline"
    fi
  fi
fi
echo ""

# =============================================================================
# CHECK 6A: Planning specialist dispatch for analyze-first modes
# =============================================================================
echo "--- Check 6A: Planning Specialist Dispatch ---"
if [[ -n "$state_workflow_mode" ]]; then
  planning_required_agents=()
  spec_file="$feature_dir/spec.md"
  case "$state_workflow_mode" in
    product-to-delivery|improve-existing)
      planning_required_agents=("bubbles.analyst" "bubbles.design" "bubbles.plan")
      if [[ -f "$spec_file" ]] && grep -qE '^## UI Wireframes' "$spec_file"; then
        planning_required_agents+=("bubbles.ux")
      fi
      ;;
  esac

  if [[ ${#planning_required_agents[@]} -gt 0 ]]; then
    execution_history_agents="$({
      python3 -c "import json; data=json.load(open('$state_file')); execution=(data.get('execution') or {}); history=(execution.get('executionHistory') or data.get('executionHistory') or []); print('\\n'.join((entry.get('agent') or '') for entry in history if isinstance(entry, dict) and entry.get('agent')))"
    } || true)"

    missing_planning_agents=0
    for planning_agent in "${planning_required_agents[@]}"; do
      if printf '%s\n' "$execution_history_agents" | grep -qx "$planning_agent"; then
        pass "Planning specialist '$planning_agent' recorded in executionHistory"
      else
        fail "Planning specialist '$planning_agent' missing from executionHistory (workflow may have bypassed required dispatch)"
        missing_planning_agents=$((missing_planning_agents + 1))
      fi
    done

    if [[ "$missing_planning_agents" -gt 0 ]]; then
      fail "$missing_planning_agents planning specialist dispatch record(s) missing — planning-first workflow compliance not proven"
    fi
  else
    info "No planning-specialist dispatch requirement for mode '$state_workflow_mode'"
  fi
else
  info "No workflow mode recorded; skipping planning-specialist dispatch check"
fi
echo ""

# =============================================================================
# CHECK 6B: Phase-claim provenance — cross-reference completedPhaseClaims
# against executionHistory agent identity (Gate G022 extension)
# =============================================================================
echo "--- Check 6B: Phase-Claim Provenance (Gate G022 extension) ---"
if [[ -n "$state_workflow_mode" ]]; then
  # Extract executionHistory block (array of entries with agent + phasesExecuted
  # + optional provenanceMode/expandedBy/expansionReason/expansionEvidenceRef).
  # Emits one line per (agent, phase) with provenanceMode and parent-expansion metadata.
  execution_history_block="$({
    python3 -c "
import json, sys, os
spec_dir = os.path.dirname('$state_file')
with open('$state_file') as f:
    data = json.load(f)
history = data.get('execution', {}).get('executionHistory', data.get('executionHistory', []))
for entry in history:
    agent = entry.get('agent', '')
    phases = entry.get('phasesExecuted', [])
    provenance = entry.get('provenanceMode', 'specialist')
    expanded_by = entry.get('expandedBy', '')
    reason = (entry.get('expansionReason', '') or '').replace('\\t', ' ').replace('\\n', ' ')
    ev_ref = (entry.get('expansionEvidenceRef', '') or '').replace('\\t', ' ')
    for p in phases:
        print(f'{agent}\\t{p}\\t{provenance}\\t{expanded_by}\\t{reason}\\t{ev_ref}')
" 2>/dev/null
  } || true)"

  if [[ -n "$execution_history_block" ]]; then
    claimed_phases="$({
      python3 -c "
import json
with open('$state_file') as f:
    data = json.load(f)
claims = data.get('execution', {}).get('completedPhaseClaims', [])
certified = data.get('certification', {}).get('certifiedCompletedPhases', [])
def _phase_name(entry):
    if isinstance(entry, str):
        return entry
    if isinstance(entry, dict):
        candidate = entry.get('phase')
        if isinstance(candidate, str):
            return candidate
        candidate = entry.get('name')
        if isinstance(candidate, str):
            return candidate
    return None
names = []
for entry in list(claims) + list(certified):
    resolved = _phase_name(entry)
    if resolved is not None:
        names.append(resolved)
for p in set(names):
    print(p)
" 2>/dev/null
    } || true)"

    # Orchestrator allowlist for parent-expansion (sourced from workflows.yaml is future work;
    # for now hardcode the three registered orchestrators).
    orchestrator_allowlist="bubbles.workflow bubbles.goal bubbles.sprint bubbles.iterate"
    expansion_reason_regex='runSubagent|tool unavailable|nested runtime|capability missing|parent-expand|nested workflow'
    spec_dir_for_evidence="$(dirname "$state_file")"

    if [[ -n "$claimed_phases" ]]; then
      provenance_failures=0
      while IFS= read -r claimed_phase; do
        [[ -z "$claimed_phase" ]] && continue
        expected_agent="bubbles.${claimed_phase}"
        matched="false"

        # Pass 1: specialist provenance (existing behavior)
        if echo "$execution_history_block" | awk -F'\t' -v a="$expected_agent" -v p="$claimed_phase" '$1==a && $2==p && ($3=="" || $3=="specialist") {found=1} END{exit !found}'; then
          pass "Phase '$claimed_phase' has specialist provenance from $expected_agent"
          matched="true"
        # bubbles.bug delegation shortcut for implement/test
        elif [[ "$claimed_phase" == "implement" || "$claimed_phase" == "test" ]] && echo "$execution_history_block" | awk -F'\t' -v p="$claimed_phase" '$1=="bubbles.bug" && $2==p && ($3=="" || $3=="specialist") {found=1} END{exit !found}'; then
          pass "Phase '$claimed_phase' has delegated provenance from bubbles.bug"
          matched="true"
        fi

        # Pass 2: parent-expanded provenance (new — per upstream fix proposal)
        if [[ "$matched" == "false" ]]; then
          # Find a parent-expanded entry for this phase
          pe_line="$(echo "$execution_history_block" | awk -F'\t' -v p="$claimed_phase" '$2==p && $3=="parent-expanded" {print; exit}')"
          if [[ -n "$pe_line" ]]; then
            # shellcheck disable=SC2034  # surfaced for parity with pe_* fields; consumed downstream.
            pe_agent="$(echo "$pe_line" | awk -F'\t' '{print $1}')"
            pe_expanded_by="$(echo "$pe_line" | awk -F'\t' '{print $4}')"
            pe_reason="$(echo "$pe_line" | awk -F'\t' '{print $5}')"
            pe_ev_ref="$(echo "$pe_line" | awk -F'\t' '{print $6}')"

            # Validate expandedBy in allowlist
            ob_ok="false"
            for o in $orchestrator_allowlist; do
              if [[ "$pe_expanded_by" == "$o" ]]; then ob_ok="true"; break; fi
            done

            if [[ "$ob_ok" != "true" ]]; then
              fail "Phase '$claimed_phase' claims parent-expansion but expandedBy='$pe_expanded_by' is not a registered orchestrator: $orchestrator_allowlist (Gate G022)"
              provenance_failures=$((provenance_failures + 1))
            elif [[ -z "$pe_reason" ]] || [[ "${#pe_reason}" -lt 20 ]]; then
              fail "Phase '$claimed_phase' claims parent-expansion but expansionReason is empty or <20 chars (Gate G022). Got: '$pe_reason'"
              provenance_failures=$((provenance_failures + 1))
            elif ! echo "$pe_reason" | grep -qiE "$expansion_reason_regex"; then
              fail "Phase '$claimed_phase' expansionReason does not name the missing capability (must mention one of: runSubagent, tool unavailable, nested runtime, capability missing, parent-expand). Got: '$pe_reason' (Gate G022)"
              provenance_failures=$((provenance_failures + 1))
            elif [[ -z "$pe_ev_ref" ]]; then
              fail "Phase '$claimed_phase' claims parent-expansion but expansionEvidenceRef is empty (Gate G022)"
              provenance_failures=$((provenance_failures + 1))
            else
              # Resolve evidence ref: relative to spec dir, repo root, or absolute
              ev_resolved=""
              for candidate in "$pe_ev_ref" "$spec_dir_for_evidence/$pe_ev_ref" "$(pwd)/$pe_ev_ref"; do
                # Strip optional #anchor suffix for file existence check
                candidate_path="${candidate%%#*}"
                if [[ -f "$candidate_path" ]]; then
                  ev_resolved="$candidate_path"
                  break
                fi
              done
              if [[ -z "$ev_resolved" ]]; then
                fail "Phase '$claimed_phase' expansionEvidenceRef='$pe_ev_ref' does not resolve to a file (Gate G022)"
                provenance_failures=$((provenance_failures + 1))
              else
                pass "Phase '$claimed_phase' has parent-expanded provenance from $pe_expanded_by — INFO[G022-PARENT-EXPANDED] reason: $pe_reason → $ev_resolved"
                matched="true"
              fi
            fi
          fi
        fi

        if [[ "$matched" != "true" ]]; then
          fail "Phase '$claimed_phase' is in completedPhaseClaims but no specialist or parent-expanded provenance found (Gate G022)"
          provenance_failures=$((provenance_failures + 1))
        fi
      done <<< "$claimed_phases"
      if [[ "$provenance_failures" -gt 0 ]]; then
        fail "$provenance_failures phase claim(s) lack proper agent provenance — phase impersonation detected"
      fi
    else
      info "No phase claims to verify provenance for"
    fi
  else
    info "No executionHistory found — phase provenance check skipped (state.json may be legacy format)"
  fi
fi
echo ""

# =============================================================================
# CHECK 7: Timestamp plausibility — detect uniformly-spaced timestamps
# =============================================================================
echo "--- Check 7: Timestamp Plausibility ---"
timestamps=()
while IFS= read -r ts; do
  timestamps+=("$ts")
done < <(grep -oE '"completedAt"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" 2>/dev/null \
  | sed -E 's/.*"completedAt"[[:space:]]*:[[:space:]]*"([^"]+)"/\1/' || true)

if [[ ${#timestamps[@]} -ge 3 ]]; then
  # Convert timestamps to epoch seconds and check intervals
  prev_epoch=0
  intervals=()
  all_parseable="true"
  for ts in "${timestamps[@]}"; do
    epoch="$(bubbles_iso_to_epoch "$ts" || true)"
    if [[ -z "$epoch" ]]; then
      all_parseable="false"
      break
    fi
    if [[ "$prev_epoch" -gt 0 ]]; then
      interval=$((epoch - prev_epoch))
      intervals+=("$interval")
    fi
    prev_epoch="$epoch"
  done

  if [[ "$all_parseable" == "true" ]] && [[ ${#intervals[@]} -ge 2 ]]; then
    # Check if all intervals are identical (suspicious uniform spacing)
    all_identical="true"
    first_interval="${intervals[0]}"
    for interval in "${intervals[@]}"; do
      if [[ "$interval" -ne "$first_interval" ]]; then
        all_identical="false"
        break
      fi
    done

    if [[ "$all_identical" == "true" ]]; then
      fail "All completion timestamps have identical intervals (${first_interval}s apart) — FABRICATION INDICATOR"
      info "Timestamps: ${timestamps[*]}"
    else
      pass "Timestamp intervals are variable (not uniformly fabricated)"
    fi

    # Check if all timestamps are within 1 second of each other
    min_epoch="$(bubbles_iso_to_epoch "${timestamps[0]}" || true)"
    max_epoch="$min_epoch"
    for ts in "${timestamps[@]}"; do
      epoch="$(bubbles_iso_to_epoch "$ts" || true)"
      [[ -n "$epoch" ]] || continue
      [[ "$epoch" -lt "$min_epoch" ]] && min_epoch="$epoch"
      [[ "$epoch" -gt "$max_epoch" ]] && max_epoch="$epoch"
    done
    spread=$((max_epoch - min_epoch))
    if [[ "$spread" -lt 5 ]] && [[ ${#timestamps[@]} -ge 3 ]]; then
      fail "All ${#timestamps[@]} phase timestamps span only ${spread}s — impossible for real sequential execution"
    fi
  fi
elif [[ ${#timestamps[@]} -eq 0 ]]; then
  warn "No completedAt timestamps found in state.json"
else
  info "Only ${#timestamps[@]} timestamp(s) found — skipping interval analysis"
fi
echo ""

# =============================================================================
# CHECK 7A: executionHistory timestamp plausibility (Gate G077)
# =============================================================================
# The convergence-loop modes (full-delivery, bugfix-fastlane) produce many
# executionHistory entries with runStartedAt/runCompletedAt. Detect:
#   (a) uniform inter-entry intervals (e.g. exactly 15 minutes apart)
#   (b) zero-duration entries (start == end) for non-trivial phases
#   (c) overlapping entries (one agent's run overlaps the next)
echo "--- Check 7A: executionHistory Timestamp Plausibility ---"
exec_history_analysis="$(python3 - "$state_file" <<'PY'
import json
import sys
from datetime import datetime

ZERO_DURATION_EXEMPT = {"finalize", "select"}

def parse_ts(value):
    if not isinstance(value, str) or not value:
        return None
    try:
        # Allow trailing Z
        if value.endswith("Z"):
            value = value[:-1] + "+00:00"
        return datetime.fromisoformat(value)
    except Exception:
        return None

try:
    with open(sys.argv[1], encoding="utf-8") as fh:
        data = json.load(fh)
except Exception:
    sys.exit(0)

history = []
container = data.get("execution", {}) if isinstance(data.get("execution"), dict) else data
raw = container.get("executionHistory", [])
if not isinstance(raw, list):
    raw = []

entries = []
for entry in raw:
    if not isinstance(entry, dict):
        continue
    started = parse_ts(entry.get("runStartedAt"))
    completed = parse_ts(entry.get("runCompletedAt"))
    if started is None or completed is None:
        continue
    phases = entry.get("phasesExecuted") or []
    if not isinstance(phases, list):
        phases = []
    entries.append({
        "agent": entry.get("agent", "<unknown>"),
        "started": started,
        "completed": completed,
        "phases": [p for p in phases if isinstance(p, str)],
    })

if len(entries) < 3:
    print(f"COUNT={len(entries)}")
    sys.exit(0)

entries.sort(key=lambda e: e["started"])
print(f"COUNT={len(entries)}")

# Check uniform intervals between consecutive runStartedAt timestamps
intervals = []
for i in range(1, len(entries)):
    intervals.append(int((entries[i]["started"] - entries[i-1]["started"]).total_seconds()))
if intervals and len(set(intervals)) == 1 and intervals[0] > 0:
    print(f"UNIFORM_INTERVAL={intervals[0]}")

# Check zero-duration entries (excluding intentionally zero phases)
zero_dur_offenders = []
for e in entries:
    duration = (e["completed"] - e["started"]).total_seconds()
    if duration <= 0:
        if not e["phases"] or any(p not in ZERO_DURATION_EXEMPT for p in e["phases"]):
            zero_dur_offenders.append(f"{e['agent']}:{','.join(e['phases']) or '?'}")
if zero_dur_offenders:
    print(f"ZERO_DURATION={'|'.join(zero_dur_offenders)}")

# Check overlapping entries (entry N+1 starts before entry N ends)
overlaps = []
for i in range(1, len(entries)):
    prev = entries[i-1]
    curr = entries[i]
    if curr["started"] < prev["completed"]:
        overlaps.append(
            f"{prev['agent']}({prev['started'].isoformat()}-{prev['completed'].isoformat()}) overlaps {curr['agent']}({curr['started'].isoformat()})"
        )
if overlaps:
    print(f"OVERLAPS={len(overlaps)}")
    for line in overlaps:
        print(f"OVERLAP_DETAIL={line}")
PY
)"

exec_count="$(echo "$exec_history_analysis" | grep -E '^COUNT=' | head -n 1 | sed 's/^COUNT=//' || true)"
if [[ -z "$exec_count" ]] || [[ "$exec_count" -lt 3 ]]; then
  info "executionHistory has fewer than 3 entries — plausibility check skipped"
else
  info "executionHistory entries analyzed: $exec_count"

  uniform_interval="$(echo "$exec_history_analysis" | grep -E '^UNIFORM_INTERVAL=' | head -n 1 | sed 's/^UNIFORM_INTERVAL=//' || true)"
  if [[ -n "$uniform_interval" ]]; then
    fail "executionHistory has $exec_count entries with identical ${uniform_interval}s intervals — FABRICATION INDICATOR"
  fi

  zero_dur_line="$(echo "$exec_history_analysis" | grep -E '^ZERO_DURATION=' | head -n 1 | sed 's/^ZERO_DURATION=//' || true)"
  if [[ -n "$zero_dur_line" ]]; then
    fail "executionHistory contains zero-duration entries for non-trivial phases: $zero_dur_line"
  fi

  overlap_count="$(echo "$exec_history_analysis" | grep -E '^OVERLAPS=' | head -n 1 | sed 's/^OVERLAPS=//' || true)"
  if [[ -n "$overlap_count" ]] && [[ "$overlap_count" -gt 0 ]]; then
    fail "executionHistory contains $overlap_count overlapping entries — sequential agent execution is impossible if runs overlap"
    while IFS= read -r detail; do
      info "$detail"
    done < <(echo "$exec_history_analysis" | grep -E '^OVERLAP_DETAIL=' | sed 's/^OVERLAP_DETAIL=//')
  fi

  if [[ -z "$uniform_interval" ]] && [[ -z "$zero_dur_line" ]] && { [[ -z "$overlap_count" ]] || [[ "$overlap_count" -eq 0 ]]; }; then
    pass "executionHistory timestamps look plausible (no uniform spacing, zero-duration entries, or overlaps)"
  fi
fi
echo ""

# =============================================================================
# CHECK 7B: Lockdown round consistency
# =============================================================================
# certification.lockdownState.round is an agent-written counter. If a non-zero
# round count is claimed, executionHistory must contain enough distinct
# implement-phase entries to plausibly back that claim.
echo "--- Check 7B: Lockdown Round Consistency ---"
lockdown_summary="$(python3 - "$state_file" <<'PY'
import json
import sys

try:
    with open(sys.argv[1], encoding="utf-8") as fh:
        data = json.load(fh)
except Exception:
    sys.exit(0)

cert = data.get("certification", {})
if not isinstance(cert, dict):
    sys.exit(0)
state = cert.get("lockdownState")
if not isinstance(state, dict):
    sys.exit(0)
round_count = state.get("round", 0)
last_clean = state.get("lastCleanRound")
print(f"ROUND={round_count}")
if last_clean is not None:
    print(f"LAST_CLEAN={last_clean}")

container = data.get("execution", {}) if isinstance(data.get("execution"), dict) else data
history = container.get("executionHistory", [])
if not isinstance(history, list):
    history = []

implement_runs = 0
for entry in history:
    if not isinstance(entry, dict):
        continue
    phases = entry.get("phasesExecuted") or []
    if not isinstance(phases, list):
        continue
    if "implement" in phases:
        implement_runs += 1
print(f"IMPLEMENT_RUNS={implement_runs}")
PY
)"

if [[ -z "$lockdown_summary" ]]; then
  info "No certification.lockdownState present — lockdown round check skipped"
else
  ld_round="$(echo "$lockdown_summary" | grep -E '^ROUND=' | head -n 1 | sed 's/^ROUND=//' || true)"
  ld_last_clean="$(echo "$lockdown_summary" | grep -E '^LAST_CLEAN=' | head -n 1 | sed 's/^LAST_CLEAN=//' || true)"
  ld_implement_runs="$(echo "$lockdown_summary" | grep -E '^IMPLEMENT_RUNS=' | head -n 1 | sed 's/^IMPLEMENT_RUNS=//' || true)"

  ld_round="${ld_round:-0}"
  ld_implement_runs="${ld_implement_runs:-0}"

  if [[ "$ld_round" -gt 0 ]] && [[ "$ld_implement_runs" -lt "$ld_round" ]]; then
    fail "lockdownState.round=$ld_round but executionHistory has only $ld_implement_runs implement-phase run(s) — round counter likely fabricated"
  elif [[ -n "$ld_last_clean" ]] && [[ "$ld_last_clean" -gt "$ld_round" ]]; then
    fail "lockdownState.lastCleanRound=$ld_last_clean exceeds round=$ld_round — impossible counter state"
  else
    pass "lockdownState round=$ld_round is consistent with $ld_implement_runs implement-phase run(s) in executionHistory"
  fi
fi
echo ""

# =============================================================================
# CHECK 8: Test file existence — verify Test Plan files exist on disk
# =============================================================================
echo "--- Check 8: Test File Existence ---"
test_files_in_plan=()
for scope_path in "${scope_files[@]}"; do
  [[ -f "$scope_path" ]] || continue
  while IFS= read -r line; do
    path="$(echo "$line" | grep -oE '`[^`]+\.(spec|test|rs|ts|tsx|js|jsx)\b[^`]*`' | sed 's/`//g' | head -1 || true)"
    if [[ -n "$path" ]] && [[ "$path" != "[path]" ]] && [[ ! "$path" =~ ^\[ ]]; then
      test_files_in_plan+=("$path")
    fi
  done < <(grep -E '^\|.*\|.*\|.*\|' "$scope_path" 2>/dev/null || true)
done

missing_test_files=0
if [[ ${#test_files_in_plan[@]} -gt 0 ]]; then
  for test_path in "${test_files_in_plan[@]}"; do
    if [[ -f "$test_path" ]]; then
      pass "Test file exists: $test_path"
    elif [[ "$test_path" != */* ]]; then
      unique_match="$({ bubbles_pruned_find "$feature_dir/../.." -type f -name "$test_path" -print 2>/dev/null; } || true)"
      unique_match_count="$({ printf '%s\n' "$unique_match" | grep -c .; } || true)"
      if [[ "$unique_match_count" -eq 1 ]]; then
        warn "Test Plan uses basename-only path '$test_path'; uniquely resolved to $(echo "$unique_match" | sed "s#^$feature_dir/../..##")"
      else
        fail "Test Plan references non-existent or non-resolvable file: $test_path"
        missing_test_files=$((missing_test_files + 1))
      fi
    else
      fail "Test Plan references non-existent file: $test_path"
      missing_test_files=$((missing_test_files + 1))
    fi
  done
  if [[ "$missing_test_files" -gt 0 ]]; then
    fail "$missing_test_files of ${#test_files_in_plan[@]} test files from Test Plan DO NOT EXIST"
  fi
else
  warn "No concrete test file paths found in Test Plan across resolved scope files (all may be placeholders)"
fi

# =============================================================================
# CHECKS 8A-8D: regression-E2E planning, consumer trace (G043), shared-infra
# blast-radius (G067), and change-boundary containment (G069). Extracted to a
# guards/ fragment (M4 split) and sourced in this shell scope (byte-identical).
# =============================================================================
source "$SCRIPT_DIR/guards/planning-checks.sh"

# =============================================================================
# CHECK 9: Evidence depth — DoD [x] items must have evidence blocks
# =============================================================================
echo "--- Check 9: DoD Evidence Presence ---"
checked_without_evidence=0
checked_with_evidence=0

# v5.2 / F1: Tool-log primary evidence path. Returns 0 (covers DoD) when
# the spec's tool-call log contains an entry whose `cmd` shares ≥2 distinct
# alpha-tokens with the DoD line body AND `exitCode == 0`. Returns 1 otherwise.
#
# Safe to call even when no log exists or python3 is unavailable (returns 1).
# The decision is local — we do NOT mutate anti-fabrication policy:
#   - Markdown evidence paths (cases 1-3) remain valid.
#   - When neither markdown nor tool-log covers the item, fail (case 4 else).
#
# Cheap matcher; v6 will replace with MCP query_tool_log RPC.
_tool_log_covers_dod_item() {
  local scope_dir="$1"
  local dod_line="$2"
  command -v python3 >/dev/null 2>&1 || return 1
  # Resolve repo root from the scope dir.
  local repo_root
  repo_root="$(cd "$scope_dir" && git rev-parse --show-toplevel 2>/dev/null || pwd)"
  local log_path="$repo_root/.specify/runtime/tool-calls.jsonl"
  [[ -f "$log_path" ]] || return 1
  local spec_slug
  spec_slug="$(basename "$(cd "$scope_dir" && (cd .. 2>/dev/null && pwd) || pwd)")"
  # If the scope_dir IS the spec dir (single-file mode), use its basename.
  if [[ -f "$scope_dir/scopes.md" || -f "$scope_dir/spec.md" ]]; then
    spec_slug="$(basename "$scope_dir")"
  fi
  SCOPE_DIR="$scope_dir" SPEC_SLUG="$spec_slug" LOG_PATH="$log_path" DOD_LINE="$dod_line" \
    python3 - <<'PY'
import json, os, re, sys
log_path = os.environ['LOG_PATH']
spec_slug = os.environ['SPEC_SLUG']
dod = os.environ['DOD_LINE']

# Tokenize DoD body (lower, strip leading `- [x] `, keep alpha-num/dot/slash/dash tokens).
body = re.sub(r'^- \[x\] ', '', dod)
toks_re = re.compile(r'[a-zA-Z][a-zA-Z0-9._/-]{2,}')
STOP = {'the','and','for','with','this','that','from','into','have','test','tests','file','files','code','docs','doc'}
dod_toks = {t.lower() for t in toks_re.findall(body)} - STOP
if len(dod_toks) < 2:
    sys.exit(1)

try:
    with open(log_path) as f:
        for raw in f:
            raw = raw.strip()
            if not raw:
                continue
            try:
                d = json.loads(raw)
            except Exception:
                continue
            # Match this spec OR framework-level entries.
            sf = (d.get('spec') or '').strip()
            if sf and sf != spec_slug and not sf.startswith(spec_slug.split('-', 1)[0]):
                continue
            if d.get('exitCode') != 0:
                continue
            cmd = (d.get('cmd') or '').lower()
            cmd_toks = {t.lower() for t in toks_re.findall(cmd)} - STOP
            if len(dod_toks & cmd_toks) >= 2:
                sys.exit(0)
except FileNotFoundError:
    sys.exit(1)
sys.exit(1)
PY
}

# v4.1.0: Evidence-by-reference resolver. When a DoD line is shaped like
#   - [x] Item description → Evidence: [anchor-name](report.md#anchor-name)
# follow the link to the report.md anchor and verify a ≥10-line evidence
# block exists between the anchor heading and the next heading (or EOF).
# This honors the long-standing report.md convention where multi-line
# terminal output is captured ONCE in report.md and referenced from many
# DoD items, instead of inlined 10+ lines under each [x] (which would
# bloat scopes.md without adding evidence value).
resolve_evidence_by_reference() {
  local scope_dir="$1"
  local link_target="$2"     # e.g. "report.md#scope-3-cosign"
  local rel_report="${link_target%%#*}"
  local anchor="${link_target##*#}"
  [[ -z "$anchor" || "$anchor" == "$link_target" ]] && return 1
  # Resolve report path relative to scope file's directory
  local report_path
  if [[ "$rel_report" == /* ]]; then
    report_path="$rel_report"
  else
    report_path="$scope_dir/$rel_report"
  fi
  [[ -f "$report_path" ]] || return 1
  # Normalize anchor: GitHub-style slugify (lower, spaces->dash, strip non-alnum/dash)
  local anchor_lower
  anchor_lower="$(echo "$anchor" | tr '[:upper:]' '[:lower:]')"
  # Find the anchor — match either an HTML anchor <a name="X">, an explicit
  # {#anchor} attribute, or a Markdown heading whose GitHub slug matches.
  local anchor_line
  anchor_line="$(awk -v a="$anchor_lower" '
    BEGIN { IGNORECASE=1 }
    /<a[[:space:]]+name=/ {
      if (tolower($0) ~ "name=\""a"\"") { print NR; exit }
    }
    /\{#[^}]+\}/ {
      if (tolower($0) ~ "\\{#"a"\\}") { print NR; exit }
    }
    /^#+[[:space:]]/ {
      h = $0
      sub(/^#+[[:space:]]+/, "", h)
      sub(/[[:space:]]+\{#[^}]+\}[[:space:]]*$/, "", h)
      slug = tolower(h)
      gsub(/[^a-z0-9 -]/, "", slug)
      gsub(/[[:space:]]+/, "-", slug)
      if (slug == a) { print NR; exit }
    }
  ' "$report_path")"
  [[ -z "$anchor_line" ]] && return 1
  # Count non-blank lines from anchor_line+1 until next heading or EOF
  local end_line
  end_line="$(awk -v start="$anchor_line" 'NR>start && /^#+[[:space:]]/ { print NR; exit }' "$report_path")"
  [[ -z "$end_line" ]] && end_line="$(wc -l < "$report_path")"
  local block_lines
  block_lines="$(sed -n "$((anchor_line+1)),${end_line}p" "$report_path" | grep -cE '\S' || true)"
  if [[ "${block_lines:-0}" -ge 10 ]]; then
    return 0
  fi
  return 1
}

# BUG-005: precompiled patterns for the per-[x]-DoD-item evidence-marker scan
# (bash builtins replace echo|grep forks). `_c9_evidence_marker_re` is matched
# case-INSENSITIVELY (under nocasematch) to mirror the original grep -qiE; the
# link/inline patterns are case-SENSITIVE (original grep -qE/-qoE).
_c9_evidence_marker_re='(→[[:space:]]*Evidence:|Evidence:)'
_c9_report_link_re='\[[^]]+\]\([^)]*report\.md(#[A-Za-z0-9_.-]+)?\)'
_c9_inline_evidence_re='(Executed:|Command:|Evidence|```|Exit Code:|Raw Output)'

for scope_path in "${scope_files[@]}"; do
  [[ -f "$scope_path" ]] || continue
  scope_dir="$(dirname "$scope_path")"
  while IFS= read -r line; do
    item_line_num="$({ grep -nF -- "$line" "$scope_path" | head -1 | cut -d: -f1; } || true)"
    if [[ -n "$item_line_num" ]]; then
      next_lines="$({ sed -n "$((item_line_num+1)),$((item_line_num+15))p" "$scope_path"; } || true)"

      # BUG-005: precompute the cheap evidence-marker booleans with bash builtins
      # (was 3 echo|grep forks per [x] DoD line). `_c9_marker` is case-INSENSITIVE
      # (original grep -qiE); `_c9_link`/`_c9_inline` are case-SENSITIVE. The
      # expensive tool-log fallback in the chain below stays lazily evaluated.
      _c9_marker=0; _c9_link=0; _c9_inline=0
      shopt -s nocasematch
      [[ "$line" =~ $_c9_evidence_marker_re ]] && _c9_marker=1
      shopt -u nocasematch
      [[ "$line" =~ $_c9_report_link_re ]] && _c9_link=1
      [[ "$next_lines" =~ $_c9_inline_evidence_re ]] && _c9_inline=1

      # 1. Inline Evidence: marker on the same line
      if [[ "$_c9_marker" -eq 1 ]]; then
        # v4.1.0: if Evidence reference is a markdown link to a report
        # anchor, follow it and require ≥10-line block.
        # NOTE: `|| true` at end keeps `set -euo pipefail` from killing the
        # whole guard silently when the line has an `Evidence:` marker but
        # no `#anchor` in the link (e.g. plain `[report.md](report.md)`).
        # Without it, the inner grep exits 1, pipefail propagates, and the
        # EXIT trap fires before this branch can fall through to the plain
        # link handler below.
        link_target="$(echo "$line" | grep -oE '\[[^]]+\]\([^)]*report\.md#[A-Za-z0-9_-]+\)' | head -1 | sed -E 's/.*\(([^)]+)\)$/\1/' || true)"
        if [[ -n "$link_target" ]]; then
          if resolve_evidence_by_reference "$scope_dir" "$link_target"; then
            checked_with_evidence=$((checked_with_evidence + 1))
          else
            checked_without_evidence=$((checked_without_evidence + 1))
            fail "DoD item [x] references '$link_target' but anchor missing OR block <10 non-blank lines in $(relative_artifact_path "$scope_path"): $(echo "$line" | head -c 80)"
          fi
        else
          checked_with_evidence=$((checked_with_evidence + 1))
        fi
      # 2. v4.1.x: markdown link to report.md (with or without #anchor) on the
      # same line counts as evidence-by-reference. Anchored links are
      # additionally validated by the resolver (≥10-line block required).
      # Plain `report.md` links (no anchor) count as evidence if the file
      # exists at the expected location.
      elif [[ "$_c9_link" -eq 1 ]]; then
        # `|| true` guards against pipefail-killed silent exit on edge
        # cases where the outer grep matched but the resubstitution does
        # not (e.g. exotic link shapes).
        link_target="$(echo "$line" | grep -oE '\[[^]]+\]\([^)]*report\.md(#[A-Za-z0-9_.-]+)?\)' | head -1 | sed -E 's/.*\(([^)]+)\)$/\1/' || true)"
        if [[ "$link_target" == *"#"* ]]; then
          if resolve_evidence_by_reference "$scope_dir" "$link_target"; then
            checked_with_evidence=$((checked_with_evidence + 1))
          else
            checked_without_evidence=$((checked_without_evidence + 1))
            fail "DoD item [x] links '$link_target' but anchor missing OR block <10 non-blank lines in $(relative_artifact_path "$scope_path"): $(echo "$line" | head -c 80)"
          fi
        else
          # Plain report.md link with no anchor — verify file presence
          rel_report="${link_target##*/}"
          [[ -z "$rel_report" ]] && rel_report="report.md"
          if [[ -f "$scope_dir/report.md" ]]; then
            checked_with_evidence=$((checked_with_evidence + 1))
          else
            checked_without_evidence=$((checked_without_evidence + 1))
            fail "DoD item [x] links report.md but no report.md exists in $scope_dir: $(echo "$line" | head -c 80)"
          fi
        fi
      # 3. Inline evidence block within next 15 lines (v4.0.x behavior)
      elif [[ "$_c9_inline" -eq 1 ]]; then
        checked_with_evidence=$((checked_with_evidence + 1))
      # 4. v5.2 / F1: structured tool-log entry covers this DoD item.
      # Accept the DoD as evidenced when bubbles/scripts/evidence-tool-log-bridge.sh
      # reports a matching tool-call entry with exitCode=0 for this spec.
      # This makes tool-log a PRIMARY evidence path: agents that wrap their
      # gate-relevant commands via tool-log.sh no longer need to inline
      # ≥10-line raw output under every DoD item — the structured log is
      # cryptographic-hash-grade evidence that the command actually ran.
      # Markdown/anchor paths above remain valid for the entire v5.2 cycle.
      elif _tool_log_covers_dod_item "$scope_dir" "$line"; then
        checked_with_evidence=$((checked_with_evidence + 1))
      else
        checked_without_evidence=$((checked_without_evidence + 1))
        fail "DoD item [x] has NO evidence block in $(relative_artifact_path "$scope_path"): $(echo "$line" | head -c 80)"
      fi
    fi
  done < <(grep -E '^\- \[x\] ' "$scope_path" 2>/dev/null || true)
done

if [[ "$checked_without_evidence" -eq 0 ]] && [[ "$checked_with_evidence" -gt 0 ]]; then
  pass "All $checked_with_evidence checked DoD items across resolved scope files have evidence blocks"
elif [[ "$checked_with_evidence" -eq 0 ]] && [[ "$total_checked" -gt 0 ]]; then
  fail "ALL checked DoD items across resolved scope files lack evidence blocks — BULK FABRICATION DETECTED"
fi
echo ""

# =============================================================================
# CHECK 10: Template placeholder detection
# =============================================================================
echo "--- Check 10: Template Placeholder Detection ---"
for scope_path in "${scope_files[@]}"; do
  [[ -f "$scope_path" ]] || continue
  template_hits="$({ grep -cnE '\[ACTUAL terminal output|\[exact cmd\]|\[actual exit code\]|\[ACTUAL output|\[command \+ output|\[cmd\]|\[PASTE VERBATIM terminal output|\[PASTE VERBATIM.*output here' "$scope_path"; } || true)"
  if [[ "$template_hits" -gt 0 ]]; then
    fail "$(relative_artifact_path "$scope_path") contains $template_hits unfilled template placeholders — FABRICATION"
  else
    pass "No template placeholders in $(relative_artifact_path "$scope_path")"
  fi
done

for report_path in "${report_files[@]}"; do
  [[ -f "$report_path" ]] || continue
  report_template_hits="$({ grep -cnE '\[ACTUAL terminal output|\[exact cmd\]|\[actual exit code\]|\[ACTUAL output|\[command \+ output|\[PASTE VERBATIM terminal output|\[PASTE VERBATIM.*output here' "$report_path"; } || true)"
  if [[ "$report_template_hits" -gt 0 ]]; then
    fail "$(relative_artifact_path "$report_path") contains $report_template_hits unfilled template placeholders — FABRICATION"
  else
    pass "No template placeholders in $(relative_artifact_path "$report_path")"
  fi
done
echo ""

# =============================================================================
# CHECK 11: Report.md required sections
# =============================================================================
echo "--- Check 11: Report.md Required Sections ---"
if [[ ${#report_files[@]} -eq 0 ]]; then
  fail "No report.md files were resolved for this feature"
fi

# BUG-005: precompiled ERE patterns for the 8 evidence-signal categories used by
# the per-line legitimacy scan below. Single-quoted so every regex metacharacter
# (incl. `$`, `[`, `(`, backslashes) is literal to bash `[[ =~ ]]`. Categories
# i/ii/iv/v/vii are case-INSENSITIVE (original grep -qiE); iii/vi/viii are
# case-SENSITIVE (original grep -qE) — see the per-line tests below.
_c11_sig_i_re='(passed|failed|ok$| PASS | FAIL |test result:|Tests:.*suites|✓|✗|PASSED|FAILED)'
_c11_sig_ii_re='(exit code|Exit Code:|error\[|warning\[|Compiling |Finished |error:|warning:|WARN |ERROR |INFO )'
_c11_sig_iii_re='([a-zA-Z0-9_-]+/[a-zA-Z0-9_.-]+\.(rs|py|ts|tsx|js|go|sh|sql|toml|yaml|json|proto|md)|\./)'
_c11_sig_iv_re='(in [0-9]+(\.[0-9]+)?(s|ms|m)|elapsed|finished in|Duration|[0-9]+\.[0-9]+s$)'
_c11_sig_v_re='(cargo |npm |pytest|go test|jest |playwright|vitest|running [0-9]+ test|test result:)'
_c11_sig_vi_re='[0-9]+ (passed|failed|errors?|warnings?|skipped|ignored|tests?)'
_c11_sig_vii_re='(HTTP/|status.*[0-9]{3}|curl |GET /|POST /|PUT /|DELETE /|Content-Type)'
_c11_sig_viii_re='(^[dl-][rwx-]{9} |^[0-9]+:|^\$ |^> )'

for report_path in "${report_files[@]}"; do
  if [[ ! -f "$report_path" ]]; then
    fail "Missing report file: $(relative_artifact_path "$report_path")"
    continue
  fi

  required_headers=("^###[[:space:]]+Summary|^##[[:space:]]+Summary" "^###[[:space:]]+Completion Statement|^##[[:space:]]+Completion Statement" "^###[[:space:]]+Test Evidence|^##[[:space:]]+Test Evidence")
  for header in "${required_headers[@]}"; do
    if grep -qE "$header" "$report_path"; then
      pass "$(relative_artifact_path "$report_path") has required report section"
    else
      fail "$(relative_artifact_path "$report_path") missing required report section"
    fi
  done

  pending_placeholders="$({ grep -nE '\[PENDING[^]]*\]|header only initially|Ready for /bubbles\.|Re-run /bubbles\.validate|Commit the fix|Record DoD evidence|Run full E2E suite|^#{1,4}[[:space:]]+Next Steps|^-[[:space:]]+Next Steps|Recommended routing:|Recommended resolution:|Recommended next move' "$report_path"; } || true)"
  if [[ -n "$pending_placeholders" ]]; then
    fail "$(relative_artifact_path "$report_path") contains unresolved placeholder or manual follow-up language"
    echo "$pending_placeholders" | sed 's/^/   -> /'
  fi

  # BUG-005: zero-fork evidence-block legitimacy scan. The previous version
  # forked a subshell per line (echo|grep fence test) and 8x per closed block
  # (echo "$block_content" | grep), costing ~126s on a 4888-line report.md. All
  # per-line/per-block forks are now bash builtins; the 8 DISTINCT signal
  # CATEGORIES are accumulated as flags while reading each in-block line (zero
  # forks). The verdict is byte-identical: a block is legitimate iff it has >=3
  # lines AND >=2 DISTINCT matching categories. A naive single `grep -cE` would
  # count matching LINES (not categories) and would CHANGE the verdict, so it is
  # intentionally NOT used. Per-line testing also preserves grep's line-oriented
  # `^`/`$` anchor semantics (each line is matched on its own).
  illegitimate_blocks=0
  total_blocks=0
  in_block=0
  block_lines=0
  sig_i=0; sig_ii=0; sig_iii=0; sig_iv=0; sig_v=0; sig_vi=0; sig_vii=0; sig_viii=0
  while IFS= read -r line; do
    if [[ "$in_block" -eq 0 ]] && [[ "$line" == '```'* ]]; then
      in_block=1
      block_lines=0
      sig_i=0; sig_ii=0; sig_iii=0; sig_iv=0; sig_v=0; sig_vi=0; sig_vii=0; sig_viii=0
    elif [[ "$in_block" -eq 1 ]] && [[ "$line" == '```' ]]; then
      in_block=0
      total_blocks=$((total_blocks + 1))

      if [[ "$block_lines" -lt 3 ]]; then
        illegitimate_blocks=$((illegitimate_blocks + 1))
      else
        signals=$((sig_i + sig_ii + sig_iii + sig_iv + sig_v + sig_vi + sig_vii + sig_viii))
        if [[ "$signals" -lt 2 ]]; then
          illegitimate_blocks=$((illegitimate_blocks + 1))
        fi
      fi
    elif [[ "$in_block" -eq 1 ]]; then
      block_lines=$((block_lines + 1))
      # 8 signal categories accumulated with zero forks. `[[ ... ]] && flag=1`
      # mirrors the original `grep ... && signals++` and is set -e safe (the
      # failing test is the non-final operand of an && list). Case-SENSITIVE
      # categories (iii, vi, viii — original grep -qE) run first with nocasematch
      # OFF; case-INSENSITIVE categories (i, ii, iv, v, vii — original grep -qiE)
      # run under `shopt -s nocasematch`. The trailing `shopt -u nocasematch`
      # both restores the default and guarantees this branch ends with exit 0.
      [[ "$line" =~ $_c11_sig_iii_re ]]  && sig_iii=1
      [[ "$line" =~ $_c11_sig_vi_re ]]   && sig_vi=1
      [[ "$line" =~ $_c11_sig_viii_re ]] && sig_viii=1
      shopt -s nocasematch
      [[ "$line" =~ $_c11_sig_i_re ]]   && sig_i=1
      [[ "$line" =~ $_c11_sig_ii_re ]]  && sig_ii=1
      [[ "$line" =~ $_c11_sig_iv_re ]]  && sig_iv=1
      [[ "$line" =~ $_c11_sig_v_re ]]   && sig_v=1
      [[ "$line" =~ $_c11_sig_vii_re ]] && sig_vii=1
      shopt -u nocasematch
    fi
  done < "$report_path"

  if [[ "$total_blocks" -eq 0 ]]; then
    fail "$(relative_artifact_path "$report_path") has ZERO evidence code blocks — no execution evidence exists"
  elif [[ "$illegitimate_blocks" -gt 0 ]]; then
    warn "$(relative_artifact_path "$report_path") has $illegitimate_blocks of $total_blocks evidence blocks that lack terminal output signals (potentially fabricated)"
  else
    pass "All $total_blocks evidence blocks in $(relative_artifact_path "$report_path") contain legitimate terminal output"
  fi

  narrative_outside_blocks="$({
    awk '
      /^```/ {in_block = !in_block; next}
      !in_block && tolower($0) ~ /(all tests pass|everything works|no issues found|verified successfully|confirmed working|tests are green|builds successfully|all checks pass)/ {count++}
      END {print count+0}
    ' "$report_path"
  } || true)"
  if [[ "$narrative_outside_blocks" -gt 0 ]]; then
    warn "$(relative_artifact_path "$report_path") has $narrative_outside_blocks narrative summary phrases outside code blocks (fabrication indicator)"
  else
    pass "No narrative summary phrases detected outside code blocks in $(relative_artifact_path "$report_path")"
  fi
done
echo ""

# =============================================================================
# CHECK 12: Duplicate evidence detection
# =============================================================================
echo "--- Check 12: Duplicate Evidence Detection ---"
for scope_path in "${scope_files[@]}"; do
  [[ -f "$scope_path" ]] || continue
  evidence_hashes=()
  in_evidence=0
  current_evidence=""
  duplicate_found="false"
  while IFS= read -r line; do
    # BUG-005: bash glob builtins replace per-line echo|grep fence forks.
    if [[ "$in_evidence" -eq 0 ]] && [[ "$line" == '    ```'* ]]; then
      in_evidence=1
      current_evidence=""
    elif [[ "$in_evidence" -eq 1 ]] && [[ "$line" == '    ```' ]]; then
      in_evidence=0
      if [[ -n "$current_evidence" ]]; then
        evidence_hash="$(echo "$current_evidence" | md5sum | cut -d' ' -f1)"
        for prev_hash in "${evidence_hashes[@]}"; do
          if [[ "$evidence_hash" == "$prev_hash" ]]; then
            fail "Duplicate evidence blocks detected in $(relative_artifact_path "$scope_path") — COPY-PASTE FABRICATION"
            duplicate_found="true"
            break 2
          fi
        done
        evidence_hashes+=("$evidence_hash")
      fi
    elif [[ "$in_evidence" -eq 1 ]]; then
      current_evidence="${current_evidence}${line}"
    fi
  done < "$scope_path"

  if [[ "$duplicate_found" == "false" ]]; then
    pass "No duplicate evidence blocks in $(relative_artifact_path "$scope_path")"
  fi
done
echo ""

# =============================================================================
# CHECK 13: Run artifact lint as final cross-check
# =============================================================================
echo "--- Check 13: Artifact Lint ---"
lint_script="$SCRIPT_DIR/artifact-lint.sh"
if [[ -f "$lint_script" ]]; then
  if BUBBLES_WORKFLOWS_FILE="$workflow_registry_file" bubbles_run_with_timeout 60 bash "$lint_script" "$feature_dir" > /dev/null 2>&1; then
    pass "Artifact lint passes (exit 0)"
  elif [[ "$is_test_fixture_dir" == "true" ]]; then
    warn "Artifact lint subprocess failed for tests/fixtures target after direct guard artifact checks passed; not blocking fixture acceptance"
  else
    fail "Artifact lint FAILED — run 'bash bubbles/scripts/artifact-lint.sh $feature_dir' for details"
  fi
else
  fail "Artifact lint script not found at $lint_script"
fi
echo ""

# =============================================================================
# CHECK 13A: Artifact freshness isolation (Gate G052)
# =============================================================================
echo "--- Check 13A: Artifact Freshness Isolation (Gate G052) ---"
freshness_guard_script="$SCRIPT_DIR/artifact-freshness-guard.sh"
if [[ -f "$freshness_guard_script" ]]; then
  if bubbles_run_with_timeout 60 bash "$freshness_guard_script" "$feature_dir" > /dev/null 2>&1; then
    pass "Artifact freshness guard passes (exit 0)"
  else
    fail "Artifact freshness guard FAILED — run 'bash bubbles/scripts/artifact-freshness-guard.sh $feature_dir' for details"
  fi
else
  fail "Artifact freshness guard script not found at $freshness_guard_script"
fi
echo ""

# =============================================================================
# CHECK 13B: Implementation delta evidence (Gate G053)
# =============================================================================
echo "--- Check 13B: Implementation Delta Evidence (Gate G053) ---"
requires_impl_delta="false"
case "$state_workflow_mode" in
  full-delivery|value-first-e2e-batch|feature-bootstrap|bugfix-fastlane|chaos-hardening|harden-to-doc|gaps-to-doc|harden-gaps-to-doc|reconcile-to-doc|stabilize-to-doc|security-to-doc|regression-to-doc|simplify-to-doc|devops-to-doc|test-to-doc|chaos-to-doc|batch-implement|batch-harden|batch-gaps|batch-harden-gaps|batch-improve-existing|batch-reconcile-to-doc|product-to-delivery|improve-existing|redesign-existing|iterate|stochastic-quality-sweep)
    requires_impl_delta="true"
    ;;
esac

if [[ "$requires_impl_delta" == "true" ]]; then
  code_diff_sections=0
  code_diff_git_signals=0
  code_diff_runtime_paths=0

  for rpt_path in "${report_files[@]}"; do
    [[ -f "$rpt_path" ]] || continue

    if grep -qE '^### Code Diff Evidence' "$rpt_path"; then
      code_diff_sections=$((code_diff_sections + 1))
    fi

    if grep -qiE '(^|[[:space:]])git (diff|show|log|status)' "$rpt_path"; then
      code_diff_git_signals=$((code_diff_git_signals + 1))
    fi

    runtime_path_hits="$({
      grep -oE '[^[:space:]]+\.(rs|go|py|ts|tsx|js|jsx|dart|java|scala|sh|bash|yaml|yml|proto)' "$rpt_path" \
        | grep -viE '(^|/)(specs|docs|\.github)/|(^|/)(README|CHANGELOG)\.md$' \
        | wc -l || true
    } || true)"
    code_diff_runtime_paths=$((code_diff_runtime_paths + runtime_path_hits))
  done

  if [[ "$code_diff_sections" -eq 0 ]]; then
    fail "Implementation-bearing workflow requires '### Code Diff Evidence' in report artifacts (Gate G053)"
  elif [[ "$code_diff_git_signals" -eq 0 ]]; then
    fail "Code Diff Evidence section is missing executed git-backed proof (git diff/show/log/status) in report artifacts (Gate G053)"
  elif [[ "$code_diff_runtime_paths" -eq 0 ]]; then
    fail "Code Diff Evidence does not show any non-artifact runtime/source/config file paths — artifact-only delivery proof is insufficient (Gate G053)"
  else
    pass "Implementation delta evidence recorded with git-backed proof and non-artifact file paths (Gate G053)"
  fi
else
  info "Workflow mode '$state_workflow_mode' does not require implementation delta evidence"
fi
echo ""

# =============================================================================
# CHECK 14: TODO/FIXME/STUB markers in implementation files
# =============================================================================
echo "--- Check 14: Implementation Completeness ---"
impl_files=()
for scope_path in "${scope_files[@]}"; do
  [[ -f "$scope_path" ]] || continue
  while IFS= read -r line; do
    path="$(echo "$line" | grep -oE '`[^`]+\.(rs|ts|tsx|js|jsx|py|go|java)\b[^`]*`' | sed 's/`//g' | head -1 || true)"
    if [[ -n "$path" ]] && [[ -f "$path" ]]; then
      impl_files+=("$path")
    fi
  done < "$scope_path"
done

if [[ ${#impl_files[@]} -gt 0 ]]; then
  todo_hits=0
  for impl_file in "${impl_files[@]}"; do
    file_todos="$({ grep -cnE '(^|[^A-Za-z0-9_])(TODO|FIXME|HACK|STUB)([^A-Za-z0-9_]|$)|unimplemented!|NotImplementedError' "$impl_file"; } || true)"
    if [[ "$file_todos" -gt 0 ]]; then
      fail "Implementation file has $file_todos TODO/STUB markers: $impl_file"
      todo_hits=$((todo_hits + file_todos))
    fi
  done
  if [[ "$todo_hits" -eq 0 ]]; then
    pass "No TODO/FIXME/STUB markers in referenced implementation files"
  fi
else
  info "No implementation file paths extracted from resolved scope files (manual check advised)"
fi
echo ""
echo ""

# =============================================================================
# CHECK 15: Phase-Scope Coherence (Gate G027)
# =============================================================================
# Detects fabricated execution/certification phase claims by cross-referencing
# against completedScopes. If implementation phases (implement, test) are
# claimed but completedScopes is empty or partial, it's fabrication.
# =============================================================================
echo "--- Check 15: Phase-Scope Coherence (Gate G027) ---"
if [[ -n "$state_workflow_mode" ]]; then
  # Only check modes that involve implementation
  case "$state_workflow_mode" in
    full-delivery|value-first-e2e-batch|feature-bootstrap|bugfix-fastlane|chaos-hardening|harden-to-doc|gaps-to-doc|harden-gaps-to-doc|reconcile-to-doc|stabilize-to-doc|security-to-doc|regression-to-doc|simplify-to-doc|devops-to-doc|test-to-doc|chaos-to-doc|batch-implement|batch-harden|batch-gaps|batch-harden-gaps|batch-improve-existing|batch-reconcile-to-doc|product-to-delivery|improve-existing|redesign-existing|iterate|stochastic-quality-sweep)
      # Check if implement/test phases are claimed
      has_implement="false"
      has_test="false"
      if echo "$state_completed_phases_block" | grep -qE '"implement"'; then
        has_implement="true"
      fi
      if echo "$state_completed_phases_block" | grep -qE '"test"'; then
        has_test="true"
      fi

      if [[ "$has_implement" == "true" || "$has_test" == "true" ]]; then
        # Implementation phases claimed — completedScopes MUST be non-empty
        if [[ "$state_completed_scopes_count" -eq 0 ]]; then
          fail "Execution/certification phases claim implement/test phases but completedScopes is EMPTY — FABRICATION (Gate G027)"
          info "This means phases were recorded without any scope actually completing"
        fi

        # Implementation phases claimed — scope artifact statuses must show work done
        if [[ "$done_scopes" -eq 0 ]]; then
          fail "Execution/certification phases claim implement/test phases but ZERO scopes are marked 'Done' — FABRICATION (Gate G027)"
        fi

        # If ALL phases claimed but scopes are partial, that's suspicious
        claimed_phase_count="$(echo "$state_completed_phases_block" | grep -cE '"(implement|test|docs|validate|audit|chaos)"' || true)"
        if [[ "$claimed_phase_count" -ge 5 ]] && [[ "$done_scopes" -lt "$total_scopes" ]] && [[ "$total_scopes" -gt 0 ]]; then
          fail "Execution/certification phases claim $claimed_phase_count lifecycle phases but only $done_scopes of $total_scopes scopes are Done — PHASE-SCOPE INCOHERENCE (Gate G027)"
        fi

        # Cross-check: completedScopes count should match done_scopes count
        if [[ "$state_completed_scopes_count" -gt 0 ]] && [[ "$done_scopes" -gt 0 ]]; then
          if [[ "$state_completed_scopes_count" -ne "$done_scopes" ]]; then
            fail "completedScopes count ($state_completed_scopes_count) does not match artifact Done count ($done_scopes) — PHASE-SCOPE INCOHERENCE (Gate G027)"
          else
            pass "completedScopes ($state_completed_scopes_count) matches artifact Done scopes ($done_scopes)"
          fi
        fi
      fi

      # If completedScopes > 0 but implement phase not claimed, that's also incoherent
      if [[ "$state_completed_scopes_count" -gt 0 ]] && [[ "$has_implement" == "false" ]]; then
        warn "completedScopes has $state_completed_scopes_count entries but 'implement' phase is missing from execution/certification phase records"
      fi

      if [[ "$has_implement" == "true" ]] && [[ "$done_scopes" -gt 0 ]] && [[ "$state_completed_scopes_count" -gt 0 ]]; then
        pass "Phase-Scope coherence verified: implementation phases align with completed scopes"
      fi
      ;;
    *)
      info "Workflow mode '$state_workflow_mode' does not require phase-scope coherence check"
      ;;
  esac
fi
echo ""

# =============================================================================
# CHECK 16: Implementation Reality Scan (Gate G028)
# =============================================================================
# Runs implementation-reality-scan.sh to detect stub/fake/hardcoded
# data patterns in source files referenced by scope artifacts.
# =============================================================================
echo "--- Check 16: Implementation Reality Scan (Gate G028) ---"
reality_scan_script="$SCRIPT_DIR/implementation-reality-scan.sh"
if [[ -f "$reality_scan_script" ]]; then
  # Only run for modes that involve implementation
  run_reality_scan="false"
  case "$state_workflow_mode" in
    full-delivery|value-first-e2e-batch|feature-bootstrap|bugfix-fastlane|chaos-hardening|harden-to-doc|gaps-to-doc|harden-gaps-to-doc|reconcile-to-doc|stabilize-to-doc|security-to-doc|regression-to-doc|simplify-to-doc|devops-to-doc|test-to-doc|chaos-to-doc|batch-implement|batch-harden|batch-gaps|batch-harden-gaps|batch-improve-existing|batch-reconcile-to-doc|product-to-delivery|improve-existing|redesign-existing|iterate|stochastic-quality-sweep)
      run_reality_scan="true"
      ;;
  esac

  if [[ "$run_reality_scan" == "true" ]]; then
    reality_output="$(bubbles_run_with_timeout 120 bash "$reality_scan_script" "$feature_dir" --verbose 2>&1 || true)"
    # shellcheck disable=SC2034  # captured for symmetry; reality_output drives the checks.
    reality_exit="$?"

    # Show condensed output
    violation_count="$(echo "$reality_output" | grep -c '🔴 VIOLATION' || true)"
    if [[ "$violation_count" -gt 0 ]]; then
      fail "Implementation reality scan found $violation_count source code violation(s) — STUB/FAKE DATA DETECTED (Gate G028)"
      # Show first 10 violations
      echo "$reality_output" | grep '🔴 VIOLATION' | head -10
      if [[ "$violation_count" -gt 10 ]]; then
        info "... and $((violation_count - 10)) more violation(s). Run 'bash $reality_scan_script $feature_dir --verbose' for full details."
      fi
    else
      pass "Implementation reality scan passed — no stub/fake/hardcoded data patterns detected"
    fi
  else
    info "Workflow mode '$state_workflow_mode' does not require implementation reality scan"
  fi
else
  fail "Implementation reality scan script not found at $reality_scan_script — cannot enforce Gate G028"
fi
echo ""

# =============================================================================
# CHECK 17: Strict mode commit enforcement (commit-per-spec)
# =============================================================================
echo "--- Check 17: Strict Mode Commit Enforcement ---"
if [[ "$state_workflow_mode" == "full-delivery" ]] && [[ "$state_status" == "done" ]]; then
  if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    spec_basename="$(basename "$feature_dir")"
    spec_id="${spec_basename%%-*}"

    feature_commit_count="$(git log --oneline -- "$feature_dir" 2>/dev/null | wc -l | tr -d ' ')"
    if [[ "$feature_commit_count" -eq 0 ]]; then
      fail "full-delivery requires at least one commit touching $feature_dir (none found)"
    else
      pass "Found $feature_commit_count commit(s) touching $feature_dir"
    fi

    structured_commit_count="$(git log --format='%s' -- "$feature_dir" 2>/dev/null | grep -Ec "^spec\(${spec_id}\)|^bubbles\(${spec_id}/" || true)"
    if [[ "$structured_commit_count" -eq 0 ]]; then
      fail "full-delivery requires at least one structured commit message for spec $spec_id (expected prefix: spec(${spec_id}) or bubbles(${spec_id}/...)"
    else
      pass "Found $structured_commit_count structured commit(s) for spec $spec_id"
    fi
  else
    fail "full-delivery commit enforcement requires execution inside a git worktree"
  fi
else
  info "Strict-mode commit enforcement not required for workflowMode '$state_workflow_mode' with status '$state_status'"
fi
echo ""

# =============================================================================
# CHECK 18: Deferral Language Scan (Gate G040)
# =============================================================================
# Scans scope artifacts for deferral language that indicates incomplete work.
# Agents that write deferral language and then mark specs "done" produce
# fabricated completion. This is the mechanical enforcement layer.
#
# Refined per spec 001-stg-check18-deferral-regex-refinement:
#   (i)  Schema-canonical follow-up field names (followUpOwner,
#        followUpAction, followUpTarget, followUps) are added to the
#        exclusion pattern. They are mandated by completion-governance.md
#        and must never count as deferral prose.
#   (ii) When state.json status is legacy read-only "done_with_concerns"
#        and legacyStatusCompatibility:true is present, the entire check is
#        skipped for compatibility. New done_with_concerns writes are blocked
#        by Gate G092.
#   (iii) Content between <!-- bubbles:g040-skip-begin --> and
#        <!-- bubbles:g040-skip-end --> HTML-comment markers is excluded
#        from the scan, letting governance docs / post-mortems quote
#        follow-up narrative inline without flipping spec status.
# =============================================================================
echo "--- Check 18: Deferral Language Scan (Gate G040) ---"

if [[ "$state_status" == "done_with_concerns" && "$(json_first_bool "legacyStatusCompatibility" "$state_file" || true)" == "true" ]]; then
  info "Check 18 skipped: state.json status is legacy read-only 'done_with_concerns' with legacyStatusCompatibility:true (Gate G040/G092)"
else
  deferral_pattern='deferred|defer to|deferred to|future scope|future work|future iteration|follow-up|follow up|followup|out of scope|not in scope|beyond scope|will address later|address later|revisit later|separate ticket|separate issue|separate PR|tracked separately|handled separately|punt\b|punted|postpone|postponed|skip for now|skipped for now|not implemented yet|not yet implemented|placeholder|temporary workaround'
  # Strategy (i): exclude schema-canonical follow-up field names mandated
  # by completion-governance.md AND the canonical "Follow-Up Narrative"
  # section heading itself. Both are schema-structural usage, not deferred-
  # work prose. grep -ivE is case-insensitive so all case variants
  # (followupowner, FollowUpOwner, follow-up narrative, FOLLOW-UP
  # NARRATIVE, etc.) are covered.
  #
  # v4.1.0: lockdownContract.patterns allowlist. When a deferral-language
  # line carries a lockdown tag from workflows.yaml.lockdownContract.patterns
  # the line is honest deferral (external actor gating runtime evidence)
  # and exits G040 cleanly. The tags themselves embed the FR citation
  # (e.g. [lockdown-deferred-FR-020]) so the schema-level requiredFields
  # contract is satisfied by the tag itself. For [awaiting-*] tags the
  # author MUST still cite the FR / condition / unblocker / expectedActivation
  # nearby — that contract is enforced by skill/instruction docs and via
  # routine artifact-lint review, not by this regex (multi-line context
  # analysis would slow the guard substantially).
  deferral_exclusion_pattern='no deferred items|no deferred work|no deferrals|without deferred work|zero deferred items|zero deferrals|no issues deferred|no issues deferred or skipped|followUpOwner|followUpAction|followUpTarget|followUps|follow-up narrative|follow-up section|\[lockdown-deferred-fr-[0-9]+\]|\[lockdown-deferred-[a-z0-9-]+-fr-[0-9]+\]|\[awaiting-operator-commit\]|\[awaiting-third-party-approval\]|\[awaiting-cutover-window\]|\[awaiting-regulator-review\]'
  total_deferral_hits=0

  # Strategy (iii): the awk filter strips fenced code AND content between
  # bubbles:g040-skip-begin / bubbles:g040-skip-end sentinel markers.
  # Marker lines themselves are dropped via `next` so they are never fed
  # to the grep.
  deferral_strip_awk='
    /^```/ || /^    ```/ { in_block = !in_block; next }
    /<!-- bubbles:g040-skip-begin -->/ { skip = 1; next }
    /<!-- bubbles:g040-skip-end -->/ { skip = 0; next }
    !in_block && !skip { print }
  '

  for scope_path in "${scope_files[@]}"; do
    [[ -f "$scope_path" ]] || continue

    # Count deferral language hits (case-insensitive), excluding inside code fence blocks
    # We scan outside code blocks only to avoid false positives from test descriptions or docs
    deferral_hits="$({
      awk "$deferral_strip_awk" "$scope_path" | grep -iE "$deferral_pattern" | grep -viE "$deferral_exclusion_pattern" | wc -l || true
    } || true)"

    if [[ "$deferral_hits" -gt 0 ]]; then
      fail "Scope artifact contains $deferral_hits deferral language hit(s): ${scope_path#$feature_dir/} — SPEC CANNOT BE DONE WITH DEFERRED WORK (Gate G040)"
      fun_message deferral_blocks_done
      total_deferral_hits=$((total_deferral_hits + deferral_hits))

      # Show first 5 matching lines for visibility
      shown_lines=0
      while IFS= read -r deferral_line; do
        [[ -n "$deferral_line" ]] || continue
        echo "   → $deferral_line"
        shown_lines=$((shown_lines + 1))
        if [[ "$shown_lines" -ge 5 ]]; then
          break
        fi
      done < <(awk "$deferral_strip_awk" "$scope_path" | grep -iE "$deferral_pattern" | grep -viE "$deferral_exclusion_pattern" || true)
    fi
  done

  # Also scan report files for deferral language
  for rpt_path in "${report_files[@]}"; do
    [[ -f "$rpt_path" ]] || continue
    report_deferral_hits="$({
      awk "$deferral_strip_awk" "$rpt_path" | grep -iE "$deferral_pattern" | grep -viE "$deferral_exclusion_pattern" | wc -l || true
    } || true)"

    if [[ "$report_deferral_hits" -gt 0 ]]; then
      fail "Report artifact contains $report_deferral_hits deferral language hit(s): ${rpt_path#$feature_dir/} — evidence of deferred work (Gate G040)"
      total_deferral_hits=$((total_deferral_hits + report_deferral_hits))
    fi
  done

  if [[ "$total_deferral_hits" -eq 0 ]]; then
    pass "Zero deferral language found in scope and report artifacts (Gate G040)"
  fi
fi
echo ""

# =============================================================================
# CHECK 19: Test Environment Dependency Detection (Gate G051)
# =============================================================================
# Scans report.md evidence for test failures caused by missing environment
# variables. These are pre-existing failures that silently undermine test
# confidence — tests pass in some environments but fail in others.
# =============================================================================
echo "--- Check 19: Test Environment Dependency Detection (Gate G051) ---"
# Generic env-dependency patterns — projects can extend via bubbles-project.yaml
env_dep_pattern='missing.*env\|env.*not set\|env.*not found\|required env\|environment variable.*missing\|panicked.*env\|config.*parse.*fail\|connection refused.*localhost\|could not connect\|cannot connect\|missing required.*config'

# Load project-specific env dependency patterns if available
PROJECT_CONFIG=".github/bubbles-project.yaml"
if [[ -f "$PROJECT_CONFIG" ]]; then
  extra_env_pattern="$(sed -n '/scans:/,/^[^ ]/{ /testEnvDependency:/,/^    [^ ]/{/patterns:/s/.*patterns:[[:space:]]*//p}}' "$PROJECT_CONFIG" 2>/dev/null || true)"
  if [[ -n "$extra_env_pattern" ]]; then
    env_dep_pattern="${env_dep_pattern}\|${extra_env_pattern}"
  fi
fi
env_dep_hits=0

for rpt_path in "${report_files[@]}"; do
  [[ -f "$rpt_path" ]] || continue
  env_hits="$(grep -ciE "$env_dep_pattern" "$rpt_path" 2>/dev/null || true)"
  if [[ "$env_hits" -gt 0 ]]; then
    fail "Report contains $env_hits test failure(s) caused by missing env vars/config: ${rpt_path#$feature_dir/} — pre-existing env-dependent test failures MUST be fixed (Gate G051)"
    env_dep_hits=$((env_dep_hits + env_hits))
    # Show first 3 matching lines
    grep -iE "$env_dep_pattern" "$rpt_path" 2>/dev/null | head -3 | while IFS= read -r env_line; do
      echo "   → $env_line"
    done
  fi
done

# Also scan scope files for evidence blocks mentioning env-dependent failures
for scope_path in "${scope_files[@]}"; do
  [[ -f "$scope_path" ]] || continue
  env_evidence_hits="$(grep -ciE "$env_dep_pattern" "$scope_path" 2>/dev/null || true)"
  if [[ "$env_evidence_hits" -gt 0 ]]; then
    fail "Scope evidence contains $env_evidence_hits env-dependent test failure indicator(s): ${scope_path#$feature_dir/} (Gate G051)"
    env_dep_hits=$((env_dep_hits + env_evidence_hits))
  fi
done

if [[ "$env_dep_hits" -eq 0 ]]; then
  pass "No env-dependent test failures detected in evidence (Gate G051)"
fi
echo ""

# =============================================================================
# CHECK 20: Enhanced Evidence Similarity Detection (Gate G021)
# =============================================================================
# Extends Check 12 by detecting near-duplicate evidence blocks where ≥80%
# of non-empty lines are shared across different DoD items. This catches
# copy-paste fabrication where agents change 1-2 lines but keep the bulk
# of the evidence identical.
# (Formerly tagged G049 — consolidated into G021 anti_fabrication_gate.)
# =============================================================================
echo "--- Check 20: Evidence Similarity Detection (Gate G021) ---"
for scope_path in "${scope_files[@]}"; do
  [[ -f "$scope_path" ]] || continue

  # Collect all evidence blocks as separate entries
  evidence_blocks=()
  in_evidence=0
  current_block=""
  while IFS= read -r line; do
    if [[ "$in_evidence" -eq 0 ]] && echo "$line" | grep -qE '^    ```'; then
      in_evidence=1
      current_block=""
    elif [[ "$in_evidence" -eq 1 ]] && echo "$line" | grep -qE '^    ```$'; then
      in_evidence=0
      if [[ -n "$current_block" ]]; then
        evidence_blocks+=("$current_block")
      fi
    elif [[ "$in_evidence" -eq 1 ]]; then
      # Skip empty lines for comparison
      trimmed="$(echo "$line" | sed 's/^[[:space:]]*//')"
      if [[ -n "$trimmed" ]]; then
        current_block="${current_block}${trimmed}"$'\n'
      fi
    fi
  done < "$scope_path"

  block_count="${#evidence_blocks[@]}"
  if [[ "$block_count" -lt 2 ]]; then
    continue
  fi

  # Compare each pair of blocks for line-level overlap
  near_dup_found="false"
  for ((i=0; i<block_count-1; i++)); do
    for ((j=i+1; j<block_count; j++)); do
      block_a="${evidence_blocks[$i]}"
      block_b="${evidence_blocks[$j]}"

      # Count lines in each block
      lines_a="$(echo "$block_a" | wc -l)"
      lines_b="$(echo "$block_b" | wc -l)"
      min_lines=$((lines_a < lines_b ? lines_a : lines_b))

      if [[ "$min_lines" -lt 5 ]]; then
        continue  # Too small to compare meaningfully
      fi

      # Count shared lines (exact match)
      shared_lines=0
      while IFS= read -r a_line; do
        [[ -z "$a_line" ]] && continue
        if echo "$block_b" | grep -qF "$a_line"; then
          shared_lines=$((shared_lines + 1))
        fi
      done <<< "$block_a"

      # Calculate overlap percentage
      overlap_pct=$((shared_lines * 100 / min_lines))

      if [[ "$overlap_pct" -ge 80 ]]; then
        fail "Near-duplicate evidence blocks (${overlap_pct}% line overlap) in $(relative_artifact_path "$scope_path") — blocks $((i+1)) and $((j+1)) of $block_count share $shared_lines of $min_lines lines. LIKELY COPY-PASTE FABRICATION (Gate G021)"
        near_dup_found="true"
        break 2
      fi
    done
  done

  if [[ "$near_dup_found" == "false" ]]; then
    pass "No near-duplicate evidence blocks in $(relative_artifact_path "$scope_path") (Gate G021)"
  fi
done
echo ""

# =============================================================================
# CHECK 21: Spec Review Enforcement for Legacy-Improvement Modes (specReview policy)
# =============================================================================
echo "--- Check 21: Spec Review Enforcement (specReview policy) ---"
if [[ "$state_status" == "done" ]] && [[ -n "$state_workflow_mode" ]]; then
  spec_review_required_modes="improve-existing|reconcile-to-doc|redesign-existing|full-delivery"
  if echo "$state_workflow_mode" | grep -qE "^($spec_review_required_modes)$"; then
    if echo "$state_completed_phases_block" | grep -qE '"spec-review"'; then
      pass "Spec-review phase recorded for legacy-improvement mode '$state_workflow_mode'"
    else
      fail "Legacy-improvement mode '$state_workflow_mode' requires a spec-review phase (specReview: once-before-implement) but 'spec-review' is NOT in execution/certification phase records"
    fi
  else
    pass "Mode '$state_workflow_mode' does not require mandatory spec-review phase"
  fi
else
  pass "Spec review enforcement skipped (status is not 'done' or workflow mode not set)"
fi
echo ""

# =============================================================================
# CHECK 22: DoD-Gherkin Content Fidelity (Gate G068)
# =============================================================================
# Verifies that every Gherkin scenario's behavioral claim is faithfully
# represented by at least one DoD item in the same scope. Detects the
# failure mode where DoD items are silently rewritten by execution agents
# to match what was delivered instead of what the spec planned.
#
# Uses the same fuzzy matching approach as traceability-guard.sh:
# - Extract significant words (4+ chars, excluding stop words) from each
#   Gherkin scenario
# - Check that at least 2-3 of those words appear in at least one DoD item
# - If no DoD item preserves the scenario's behavioral claim, flag it
# =============================================================================
echo "--- Check 22: DoD-Gherkin Content Fidelity (Gate G068) ---"

# Helper: extract significant words from text (same logic as traceability-guard.sh)
stg_normalize_text() {
  local value="$1"
  value="$(printf '%s' "$value" | tr '[:upper:]' '[:lower:]')"
  value="$(printf '%s' "$value" | sed -E 's/[^a-z0-9]+/ /g; s/[[:space:]]+/ /g; s/^ //; s/ $//')"
  printf '%s' "$value"
}

stg_significant_words() {
  local text="$1"
  local normalized
  local word

  normalized="$(stg_normalize_text "$text")"
  for word in $normalized; do
    # G068 false-positive fix (v3.8.0): lowered min word length 4 -> 3 so
    # 3-letter domain words (API, DoD, SLA, CSV, CSP, JWT, SDK, CLI, CRD,
    # SBOM) are counted as significant instead of stripped as noise.
    if [[ ${#word} -lt 3 ]]; then
      continue
    fi
    # G068 false-positive fix (v3.8.0): trimmed exclusion list to TRUE stop
    # words only. Removed domain-relevant words (user, users, system, should,
    # must, have, has, will, given, after, before, where, their, there,
    # about, only) that are frequently the distinguishing words in Gherkin
    # scenario titles.
    case "$word" in
      the|are|was|were|been|being|for|from|with|and|but|not|then|else|when|while|that|this|these|those|its|into|onto|out|all|any|each|every|some|more|less|also)
        continue
        ;;
    esac
    printf '%s\n' "$word"
  done
}

stg_scenario_matches_dod() {
  local scenario="$1"
  local dod_item="$2"
  local dod_norm
  local words
  local word
  local score=0
  local word_count=0
  local half_threshold=0

  dod_norm="$(stg_normalize_text "$dod_item")"
  words="$(stg_significant_words "$scenario")"
  if [[ -z "$words" ]]; then
    [[ "$dod_norm" == *"$(stg_normalize_text "$scenario")"* ]]
    return
  fi

  while IFS= read -r word; do
    [[ -n "$word" ]] || continue
    word_count=$((word_count + 1))
    if [[ " $dod_norm " == *" $word "* ]]; then
      score=$((score + 1))
    fi
  done <<< "$words"

  # G068 false-positive fix (v3.8.0): percentage-based threshold with floor.
  # - Very small scenarios (<3 significant words): require ALL words to match
  #   so a hard >=3 floor doesn't penalize them.
  # - Larger scenarios: require BOTH (overlap >= ceil(50% * word_count))
  #   AND (overlap >= 3) — percentage threshold with absolute floor.
  if [[ "$word_count" -lt 3 ]]; then
    [[ "$score" -eq "$word_count" ]]
    return
  fi

  half_threshold=$(( (word_count + 1) / 2 ))
  [[ "$score" -ge 3 && "$score" -ge "$half_threshold" ]]
}

dod_fidelity_failures=0
dod_fidelity_total=0
for scope_index in "${!scope_analysis_files[@]}"; do
  scope_path="${scope_analysis_files[$scope_index]}"
  [[ -f "$scope_path" ]] || continue

  scope_label="$(scope_analysis_label "$scope_index")"

  # Extract Gherkin scenarios
  scope_scenarios="$(grep -E '^[[:space:]]*Scenario( Outline)?:' "$scope_path" | sed -E 's/^[[:space:]]*Scenario( Outline)?:[[:space:]]*//' || true)"
  if [[ -z "$scope_scenarios" ]]; then
    continue
  fi

  # Extract DoD items (text only, strip checkbox prefix)
  scope_dod_items="$(awk '
    /^#{1,4}.*Definition of Done|^#{1,4}.*DoD/ {in_dod=1; next}
    /^#{1,4} / {if (in_dod) exit}
    in_dod && /^- \[(x| )\] / {
      sub(/^- \[(x| )\] /, "", $0)
      print
    }
  ' "$scope_path" || true)"

  if [[ -z "$scope_dod_items" ]]; then
    continue
  fi

  while IFS= read -r scenario; do
    [[ -n "$scenario" ]] || continue
    dod_fidelity_total=$((dod_fidelity_total + 1))

    matched=0
    while IFS= read -r dod_item; do
      [[ -n "$dod_item" ]] || continue
      if stg_scenario_matches_dod "$scenario" "$dod_item"; then
        matched=1
        break
      fi
    done <<< "$scope_dod_items"

    if [[ "$matched" -eq 0 ]]; then
      fail "DoD-Gherkin content fidelity gap in $scope_label — scenario has no faithful DoD item: $(echo "$scenario" | head -c 120)"
      dod_fidelity_failures=$((dod_fidelity_failures + 1))
    fi
  done <<< "$scope_scenarios"
done

if [[ "$dod_fidelity_total" -eq 0 ]]; then
  pass "No Gherkin scenarios to check for DoD content fidelity"
elif [[ "$dod_fidelity_failures" -gt 0 ]]; then
  fail "$dod_fidelity_failures Gherkin scenario(s) have no matching DoD item — DoD may have been rewritten to match delivery instead of spec (Gate G068)"
  info "Each Gherkin scenario's behavioral claim MUST be preserved in at least one DoD item"
  info "If a DoD item was rewritten to describe different behavior, route to bubbles.plan for plan correction"
else
  pass "All $dod_fidelity_total Gherkin scenarios have faithful DoD items (Gate G068)"
fi
echo ""

# =============================================================================
# CHECKS 23-25 + 40: convergence cap (G082), compaction discipline (G083),
# pre-existing deferral block (G084), and session cap (G128, the aggregate
# sibling of G082). Extracted to a guards/ fragment (M4 split) and sourced in
# this shell scope so behavior is byte-identical; Check 40 (G128) is additive
# and a NO-OP unless a sessionBudget is recorded.
# =============================================================================
source "$SCRIPT_DIR/guards/tail-convergence-gates.sh"

# =============================================================================
# CHECK 26: Framework Dogfood Evidence Enforcement (Gate G085)
# =============================================================================
# Mechanical wrapper around bubbles/scripts/framework-dogfood-guard.sh.
# The guard is source-aware. In the Bubbles source repository, persistent
# `specs/` are forbidden and dogfood evidence comes from framework
# validation, hermetic selftests, release manifests, and downstream or
# fixture specs. In downstream/fixture repositories, the traditional
# evidence model still applies: at least one numbered spec at status
# `done` demonstrates the installed framework can drive work to
# certification.
if [[ "${BUBBLES_STATE_TRANSITION_GUARD_SELFTEST_FAST:-0}" == "1" ]]; then
  echo "--- Check 26-39: Delegated Tail Gates (selftest fast path) ---"
  info "State-transition selftest fast path enabled; delegated gates G085-G095, G097, and G098-G100 are covered by their dedicated selftests in framework-validate"
  echo ""
else
# =============================================================================
# CHECKS 26-39: delegated tail gates G085-G095, G097, and G098-G100. Extracted
# to a guards/
# fragment (M4 split) and sourced inside this else branch so behavior is
# byte-identical.
# =============================================================================
source "$SCRIPT_DIR/guards/tail-delegated-gates.sh"
fi

# =============================================================================
# FINAL VERDICT
# =============================================================================
echo "============================================================"
echo "  TRANSITION GUARD VERDICT"
echo "============================================================"
echo ""

if [[ "$failures" -gt 0 ]]; then
  echo "🔴 TRANSITION BLOCKED: $failures failure(s), $warnings warning(s)"
  echo ""
  echo "state.json status MUST NOT be set to 'done'."
  echo "Fix ALL blocking failures above before attempting promotion."
  echo ""

  if [[ "$revert_on_fail" == "true" ]] && [[ -f "$state_file" ]]; then
    echo "--- Auto-Reverting state.json (--revert-on-fail) ---"
    now_utc="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

    clear_array_key() {
      local array_key="$1"

      if ! grep -qE '"'"$array_key"'"[[:space:]]*:[[:space:]]*\[' "$state_file"; then
        return 0
      fi

      bubbles_sed_inplace -E 's/"'"$array_key"'"[[:space:]]*:[[:space:]]*\[[^]]*\]/"'"$array_key"'": []/' "$state_file"

      awk -v key="$array_key" '
        $0 ~ "\"" key "\"[[:space:]]*:[[:space:]]*\\[" {
          if ($0 ~ /\[[^]]*\]/) {
            print
            next
          }
          sub(/"[^"]+"[[:space:]]*:[[:space:]]*.*/, "\"" key "\": [],", $0)
          print
          in_array = 1
          next
        }
        in_array && /\]/ {
          in_array = 0
          next
        }
        in_array { next }
        { print }
      ' "$state_file" > "${state_file}.tmp" && mv "${state_file}.tmp" "$state_file"
    }

    # Revert status to in_progress
    bubbles_sed_inplace -E 's/"status"[[:space:]]*:[[:space:]]*"done"/"status": "in_progress"/' "$state_file"

    # Revert certification.status to in_progress if present
    awk '
      /"certification"[[:space:]]*:/ {
        print
        in_cert = 1
        next
      }
      in_cert && /"status"[[:space:]]*:[[:space:]]*"done"/ {
        sub(/"done"/, "\"in_progress\"", $0)
        print
        next
      }
      in_cert && /^[[:space:]]*}/ {
        in_cert = 0
        print
        next
      }
      { print }
    ' "$state_file" > "${state_file}.tmp" && mv "${state_file}.tmp" "$state_file"

    clear_array_key "completedScopes"
    clear_array_key "certifiedCompletedPhases"
    clear_array_key "completedPhaseClaims"
    clear_array_key "completedPhases"

    # Update lastUpdatedAt
    bubbles_sed_inplace -E 's/"lastUpdatedAt"[[:space:]]*:[[:space:]]*"[^"]+"/"lastUpdatedAt": "'"$now_utc"'"/' "$state_file"

    # Add failure record if failures array exists
    if grep -qE '"failures"[[:space:]]*:[[:space:]]*\[' "$state_file"; then
      failure_record="{\"phase\": \"transition-guard\", \"summary\": \"$failures blocking failures detected by state-transition-guard.sh\", \"detectedAt\": \"$now_utc\"}"
      # Append to failures array (simple single-line case)
      bubbles_sed_inplace -E "s|\"failures\"[[:space:]]*:[[:space:]]*\[|\"failures\": [$failure_record, |" "$state_file"
      # Clean up empty trailing comma if array was empty
      bubbles_sed_inplace -E 's/\[({[^}]+}), \]/[\1]/' "$state_file"
    fi

    echo "REVERTED: state.json status → 'in_progress'"
    echo "REVERTED: certification.status → 'in_progress' (if present)"
    echo "REVERTED: completedScopes / certifiedCompletedPhases / completedPhaseClaims / completedPhases → []"
    echo "ADDED: failure record with timestamp $now_utc"
  fi

  # ── Run project-defined custom gates (G900+) ───────────────────────
  PROJECT_CONFIG=".github/bubbles-project.yaml"
  if [[ -f "$PROJECT_CONFIG" ]]; then
    echo ""
    echo "🔍 Running project-defined gates from $PROJECT_CONFIG..."
    while IFS= read -r line; do
      script_path=$(echo "$line" | sed 's/.*script:\s*//' | tr -d '[:space:]')
      [[ -z "$script_path" ]] && continue
      full_path=".github/$script_path"
      gate_name=$(grep -B5 "script:.*$script_path" "$PROJECT_CONFIG" | grep -oE '^\s+\S+:$' | tail -1 | tr -d '[:space:]:')
      if [[ -x "$full_path" ]]; then
        echo "  Running: $gate_name ($full_path)"
        if bash "$full_path"; then
          echo "  ✅ $gate_name passed"
        else
          blocking=$(grep -A2 "script:.*$script_path" "$PROJECT_CONFIG" | grep "blocking:" | sed 's/.*blocking:\s*//' | tr -d '[:space:]')
          if [[ "$blocking" == "true" ]]; then
            fail "Project gate BLOCKED: $gate_name ($full_path)"
          else
            warn "Project gate warning: $gate_name ($full_path)"
          fi
        fi
      else
        warn "Project gate script not found or not executable: $full_path"
      fi
    done < <(grep -E '^[[:space:]]*script:' "$PROJECT_CONFIG")
  fi

  exit 1
else
  if [[ "$warnings" -gt 0 ]]; then
    echo "🟡 TRANSITION PERMITTED with $warnings warning(s)"
  else
    echo "🟢 TRANSITION PERMITTED: All checks pass ($failures failures, $warnings warnings)"
    fun_summary pass
  fi
  echo ""
  final_status_ceiling="$(resolve_workflow_status_ceiling "$state_workflow_mode" || true)"
  if [[ -n "$final_status_ceiling" && "$state_status" == "$final_status_ceiling" && "$final_status_ceiling" != "done" ]]; then
    echo "state.json is correctly set to '$state_status' for workflowMode '$state_workflow_mode'."
  elif [[ "$final_status_ceiling" == "done" ]]; then
    echo "state.json status may be set to 'done'."
  else
    echo "state.json status '$state_status' is permitted for workflowMode '$state_workflow_mode'."
  fi
  exit 0
fi
