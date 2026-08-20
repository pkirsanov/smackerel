#!/usr/bin/env bash
# Capability: done-with-concerns-outcome-state, observability-posture-and-slo-gates,
# Capability: validate-owned-certification, workflow-orchestration,
# Capability: workflow-runner-authorization
# =============================================================================
# state-transition-guard.sh
# =============================================================================
# MANDATORY guard script that MUST be executed BEFORE any state.json status
# transition to "done". This is the mechanical enforcement layer that prevents
# agents from fabricating completion status.
#
# Usage:
#   bash bubbles/scripts/state-transition-guard.sh <feature-dir> \
#     [--target-status STATUS] \
#     [--expect-workflow-mode MODE] \
#     [--expect-contract-digest sha256:HEX] \
#     [--revert-on-fail]
#
# Exit codes:
#   0 = All applicable checks pass for the registry-derived target
#   1 = One or more checks failed, transition BLOCKED
#   2 = Transition contract could not be resolved or asserted
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

# Shared DoD section parser (BUG-026): one correct tiered-DoD boundary consumed
# by Check 4A (G041 list-format policy) and Check 22 (G068 checkbox fidelity).
source "$SCRIPT_DIR/dod-section-lib.sh"

# Shared scenario->DoD matcher (BUG-004): ONE implementation of the G068 rule,
# consumed here and by traceability-guard.sh. Check 22 passes the
# structural-strict id policy; see scenario-match-lib.sh for what that means.
source "$SCRIPT_DIR/scenario-match-lib.sh"

transition_workflow_mode="UNRESOLVED"
transition_audit_profile="UNRESOLVED"
transition_target_status="UNRESOLVED"
transition_contract_digest="UNRESOLVED"
transition_target_revision="UNRESOLVED"
transition_source_edit_lockout_required="false"
transition_applicable_check_classes=()
transition_not_applicable_checks=()
transition_required_gate_ids=()
passed_gate_ids=()
failed_gate_ids=()
failed_check_ids=()
# IMP-036 SCOPE-2: parent-expansion is already gated (registered orchestrator,
# >=20-char reason naming the missing capability, resolvable evidence ref). What
# was missing is VISIBILITY: a rate nobody counts cannot show whether SCOPE-1's
# single-orchestrator rule actually reduced expansion.
parent_expanded_phases=0

list_contains() {
  local needle="$1"
  shift
  local item
  for item in "$@"; do
    if [[ "$item" == "$needle" ]]; then
      return 0
    fi
  done
  return 1
}

record_passed_gate() {
  local gate_id="$1"
  list_contains "$gate_id" ${passed_gate_ids[@]+"${passed_gate_ids[@]}"} || passed_gate_ids+=("$gate_id")
}

record_failed_gate() {
  local gate_id="$1"
  list_contains "$gate_id" ${failed_gate_ids[@]+"${failed_gate_ids[@]}"} || failed_gate_ids+=("$gate_id")
}

record_failed_check() {
  local check_id="$1"
  list_contains "$check_id" ${failed_check_ids[@]+"${failed_check_ids[@]}"} || failed_check_ids+=("$check_id")
}

record_gate_ids_from_message() {
  local outcome="$1"
  local remaining="$2"
  local gate_id
  while [[ "$remaining" =~ (G[0-9][0-9][0-9]) ]]; do
    gate_id="${BASH_REMATCH[1]}"
    if [[ "$outcome" == "pass" ]]; then
      record_passed_gate "$gate_id"
    else
      record_failed_gate "$gate_id"
    fi
    remaining="${remaining#*"$gate_id"}"
  done
}

format_result_list() {
  local first="true"
  local item
  printf '['
  for item in "$@"; do
    if [[ "$first" == "true" ]]; then
      first="false"
    else
      printf ','
    fi
    printf '%s' "$item"
  done
  printf ']'
}

emit_transition_result() {
  local verdict="$1"
  local blocking_code="$2"
  local failure_count="$3"
  local exit_status="$4"
  local gate_id
  local effective_passed_gate_ids=()
  local observed_passed_gate_ids=()
  local unevaluated_gate_ids=()

  # IMP-047 S-A. Capture the gates a check ACTUALLY recorded before the blanket
  # PASS sweep below credits every required id. The emitted passedGateIds list is
  # unchanged, so existing readers see exactly what they saw before; telemetry
  # gets the finer fact, which is the only place the difference matters.
  for gate_id in ${passed_gate_ids[@]+"${passed_gate_ids[@]}"}; do
    observed_passed_gate_ids+=("$gate_id")
  done

  if [[ "$verdict" == "PASS" ]]; then
    for gate_id in ${transition_required_gate_ids[@]+"${transition_required_gate_ids[@]}"}; do
      if ! list_contains "$gate_id" ${observed_passed_gate_ids[@]+"${observed_passed_gate_ids[@]}"}; then
        unevaluated_gate_ids+=("$gate_id")
      fi
      record_passed_gate "$gate_id"
    done
  fi
  for gate_id in ${passed_gate_ids[@]+"${passed_gate_ids[@]}"}; do
    if ! list_contains "$gate_id" ${failed_gate_ids[@]+"${failed_gate_ids[@]}"}; then
      effective_passed_gate_ids+=("$gate_id")
    fi
  done

  printf '%s\n' 'BEGIN TRANSITION_GUARD_RESULT_V1'
  printf '%s\n' 'schemaVersion: transition-guard-result/v1'
  printf 'workflowMode: %s\n' "$transition_workflow_mode"
  printf 'auditProfile: %s\n' "$transition_audit_profile"
  printf 'targetStatus: %s\n' "$transition_target_status"
  printf 'contractDigest: %s\n' "$transition_contract_digest"
  printf 'targetRevision: %s\n' "$transition_target_revision"
  printf 'applicableCheckClasses: %s\n' "$(format_result_list ${transition_applicable_check_classes[@]+"${transition_applicable_check_classes[@]}"})"
  printf 'notApplicableChecks: %s\n' "$(format_result_list ${transition_not_applicable_checks[@]+"${transition_not_applicable_checks[@]}"})"
  printf 'passedGateIds: %s\n' "$(format_result_list ${effective_passed_gate_ids[@]+"${effective_passed_gate_ids[@]}"})"
  printf 'failedGateIds: %s\n' "$(format_result_list ${failed_gate_ids[@]+"${failed_gate_ids[@]}"})"
  printf 'failedChecks: %s\n' "$(format_result_list ${failed_check_ids[@]+"${failed_check_ids[@]}"})"
  printf 'blockingCode: %s\n' "$blocking_code"
  printf 'parentExpandedPhases: %s\n' "${parent_expanded_phases:-0}"
  printf 'failureCount: %s\n' "$failure_count"
  printf 'exitStatus: %s\n' "$exit_status"
  printf 'verdict: %s\n' "$verdict"
  printf '%s\n' 'END TRANSITION_GUARD_RESULT_V1'

  # IMP-036 SCOPE-4: append-only gate-hit telemetry. Observes only; retires
  # nothing. The helper swallows its own failures so a read-only log directory
  # can never turn into a blocked commit.
  #
  # IMP-047 S-A: firing and prevention are recorded as separate facts. A gate
  # credited only by the PASS sweep above was never evaluated, so it is reported
  # as not-evaluated rather than as a pass that never happened.
  local passed_str="" failed_str="" not_evaluated_str=""
  for gate_id in ${observed_passed_gate_ids[@]+"${observed_passed_gate_ids[@]}"}; do
    list_contains "$gate_id" ${failed_gate_ids[@]+"${failed_gate_ids[@]}"} && continue
    passed_str+="$gate_id "
  done
  for gate_id in ${failed_gate_ids[@]+"${failed_gate_ids[@]}"}; do
    failed_str+="$gate_id "
  done
  for gate_id in ${unevaluated_gate_ids[@]+"${unevaluated_gate_ids[@]}"}; do
    not_evaluated_str+="$gate_id "
  done
  if [[ -f "$SCRIPT_DIR/gate-hit-log.sh" ]]; then
    if ! declare -F bubbles_gate_hit_append >/dev/null 2>&1; then
      # shellcheck disable=SC1091
      source "$SCRIPT_DIR/gate-hit-log.sh" 2>/dev/null || true
    fi
    if declare -F bubbles_gate_hit_append >/dev/null 2>&1; then
      bubbles_gate_hit_append \
        --repo-root "${guard_repo_root:-$PWD}" \
        --spec "${feature_dir:-}" \
        --mode "$transition_workflow_mode" \
        --target-status "$transition_target_status" \
        --verdict "$verdict" \
        --exit-status "$exit_status" \
        --passed "$passed_str" \
        --failed "$failed_str" \
        --not-evaluated "$not_evaluated_str" \
        --parent-expanded "${parent_expanded_phases:-0}" >/dev/null 2>&1 || true
    fi
  fi
}

block_contract() {
  local error_code="$1"
  local detail="$2"
  printf '%s: %s\n' "$error_code" "$detail" >&2
  record_failed_check contract-resolution
  emit_transition_result BLOCKED "$error_code" 1 2
  exit 2
}

if (( $# == 0 )); then
  block_contract E009-USAGE "FEATURE_DIR is required"
fi

feature_dir="$1"
shift
if [[ -z "$feature_dir" || "$feature_dir" == --* ]]; then
  block_contract E009-USAGE "FEATURE_DIR must be the first argument"
fi

revert_on_fail="false"
expect_target_status=""
expect_workflow_mode=""
expect_contract_digest=""
while (( $# > 0 )); do
  case "$1" in
    --revert-on-fail)
      revert_on_fail="true"
      shift
      ;;
    --target-status)
      (( $# >= 2 )) || block_contract E009-USAGE "--target-status requires a value"
      [[ -z "$expect_target_status" ]] || block_contract E009-USAGE "--target-status may be supplied only once"
      expect_target_status="$2"
      shift 2
      ;;
    --target-status=*)
      [[ -z "$expect_target_status" ]] || block_contract E009-USAGE "--target-status may be supplied only once"
      expect_target_status="${1#*=}"
      [[ -n "$expect_target_status" ]] || block_contract E009-USAGE "--target-status requires a value"
      shift
      ;;
    --expect-workflow-mode)
      (( $# >= 2 )) || block_contract E009-USAGE "--expect-workflow-mode requires a value"
      [[ -z "$expect_workflow_mode" ]] || block_contract E009-USAGE "--expect-workflow-mode may be supplied only once"
      expect_workflow_mode="$2"
      shift 2
      ;;
    --expect-workflow-mode=*)
      [[ -z "$expect_workflow_mode" ]] || block_contract E009-USAGE "--expect-workflow-mode may be supplied only once"
      expect_workflow_mode="${1#*=}"
      [[ -n "$expect_workflow_mode" ]] || block_contract E009-USAGE "--expect-workflow-mode requires a value"
      shift
      ;;
    --expect-contract-digest)
      (( $# >= 2 )) || block_contract E009-USAGE "--expect-contract-digest requires a value"
      [[ -z "$expect_contract_digest" ]] || block_contract E009-USAGE "--expect-contract-digest may be supplied only once"
      expect_contract_digest="$2"
      shift 2
      ;;
    --expect-contract-digest=*)
      [[ -z "$expect_contract_digest" ]] || block_contract E009-USAGE "--expect-contract-digest may be supplied only once"
      expect_contract_digest="${1#*=}"
      [[ -n "$expect_contract_digest" ]] || block_contract E009-USAGE "--expect-contract-digest requires a value"
      shift
      ;;
    *)
      block_contract E009-USAGE "unknown or policy-selecting argument: $1"
      ;;
  esac
done

if [[ ! -d "$feature_dir" ]]; then
  block_contract E009-STATE-MALFORMED "feature directory does not exist"
fi

transition_contract_resolver="$SCRIPT_DIR/transition-contract-resolver.sh"
if [[ ! -f "$transition_contract_resolver" ]]; then
  block_contract E009-REGISTRY-MISSING "transition contract resolver is unavailable"
fi

transition_contract_stdout="$(mktemp "${TMPDIR:-/tmp}/bubbles-transition-guard-contract.XXXXXX")"
transition_contract_stderr="$(mktemp "${TMPDIR:-/tmp}/bubbles-transition-guard-contract-error.XXXXXX")"
transition_resolver_args=("$feature_dir")
[[ -z "$expect_target_status" ]] || transition_resolver_args+=(--expect-target "$expect_target_status")
[[ -z "$expect_workflow_mode" ]] || transition_resolver_args+=(--expect-mode "$expect_workflow_mode")
[[ -z "$expect_contract_digest" ]] || transition_resolver_args+=(--expect-contract-digest "$expect_contract_digest")

set +e
bash "$transition_contract_resolver" "${transition_resolver_args[@]}" > "$transition_contract_stdout" 2> "$transition_contract_stderr"
transition_resolver_status=$?
set -e
if [[ "$transition_resolver_status" -ne 0 ]]; then
  transition_resolver_error=""
  IFS= read -r transition_resolver_error < "$transition_contract_stderr" || true
  rm -f "$transition_contract_stdout" "$transition_contract_stderr"
  transition_resolver_code="${transition_resolver_error%%:*}"
  if [[ ! "$transition_resolver_code" =~ ^E009-[A-Z0-9-]+$ ]]; then
    transition_resolver_code="E009-REGISTRY-MISSING"
    transition_resolver_error="E009-REGISTRY-MISSING: transition contract resolver failed without a valid E009 result"
  fi
  block_contract "$transition_resolver_code" "${transition_resolver_error#*: }"
fi
rm -f "$transition_contract_stderr"

# The auditProfile allow-list below is the exact set transition-contract-resolver.sh
# accepts (its own four-way check). Both lists MUST move together: a profile the
# resolver supports but this list omits is rejected here as MALFORMED, which
# blocks the mode outright before any applicability branch is reached.
if ! jq -e '
  .schemaVersion == "transition-contract/v1"
  and (.workflowMode | type == "string" and length > 0)
  and (.auditProfile == "planning-maturity-v1"
    or .auditProfile == "delivery-completion-v1"
    or .auditProfile == "delivery-completion-fast-v1"
    or .auditProfile == "framework-proposal-v1"
    or .auditProfile == "operational-completion-v1")
  and (.targetStatus | type == "string" and length > 0)
  and (.currentStatus | type == "string" and length > 0)
  and (.contractDigest | type == "string" and test("^sha256:[0-9a-f]{64}$"))
  and (.targetRevision | type == "string" and test("^sha256:[0-9a-f]{64}$"))
  and (.requiredGates | type == "array" and all(.[]; type == "string" and test("^G[0-9]{3}$")))
  and (.sourceEditLockoutRequired | type == "boolean")
' "$transition_contract_stdout" >/dev/null 2>&1; then
  rm -f "$transition_contract_stdout"
  block_contract E009-AUDIT-PROFILE-CONTRADICTION "transition contract resolver emitted a malformed contract"
fi

transition_workflow_mode="$(jq -r '.workflowMode' "$transition_contract_stdout")"
transition_audit_profile="$(jq -r '.auditProfile' "$transition_contract_stdout")"
transition_target_status="$(jq -r '.targetStatus' "$transition_contract_stdout")"
transition_current_status="$(jq -r '.currentStatus' "$transition_contract_stdout")"
transition_contract_digest="$(jq -r '.contractDigest' "$transition_contract_stdout")"
transition_target_revision="$(jq -r '.targetRevision' "$transition_contract_stdout")"
transition_source_edit_lockout_required="$(jq -r '.sourceEditLockoutRequired' "$transition_contract_stdout")"
while IFS= read -r gate_id; do
  [[ -n "$gate_id" ]] || continue
  transition_required_gate_ids+=("$gate_id")
done < <(jq -r '.requiredGates[]' "$transition_contract_stdout")
rm -f "$transition_contract_stdout"

# These two lists are REPORTED in the TRANSITION_GUARD_RESULT_V1 block and gate
# no check execution. That is exactly why every profile the resolver supports
# needs a branch: a supported profile with no branch keeps the empty initialiser
# and publishes `applicableCheckClasses: []`, a false declaration about itself.
case "$transition_audit_profile" in
  planning-maturity-v1)
    transition_applicable_check_classes=(universal mode-required planning-maturity)
    transition_not_applicable_checks=(Check-4-completion Check-5-all-done Check-8-file-existence Check-11-execution-evidence)
    ;;
  delivery-completion-v1 | delivery-completion-fast-v1)
    # The fast lane keeps full implement+test+validate assurance and only omits
    # the heavyweight `audit` phase (transition-contract-resolver.sh:280-286), so
    # it asserts the same completion classes as the full delivery profile.
    transition_applicable_check_classes=(universal mode-required delivery-completion)
    ;;
  framework-proposal-v1)
    # IMP-047 PD-11: audit-only, never implements, never certifies delivery
    # (transition-contract-resolver.sh:264-278 requires audit_only, forbids
    # implement/test, and pins the ceiling to framework_proposal_written), so no
    # completion class is true for it.
    transition_applicable_check_classes=(universal mode-required)
    transition_not_applicable_checks=(Check-4-completion Check-5-all-done Check-8-file-existence Check-11-execution-evidence)
    ;;
  operational-completion-v1)
    # BUG-017: an operational or docs terminal (validated, docs_updated,
    # train_*, backup_verified, propagated_forward, incident_mitigated, ...).
    # The resolver pins its ceiling BELOW `done` and away from the planning and
    # framework-proposal ceilings, so this mode terminates at an operational
    # outcome and never certifies delivery — asserting delivery-completion here
    # would be a claim it cannot satisfy. Its exclusion list stays DECLARATIVE:
    # check_is_applicable below enforces exclusions for framework-proposal-v1
    # only, so adding this profile cannot admit any packet the live checks
    # refuse today.
    transition_applicable_check_classes=(universal mode-required)
    transition_not_applicable_checks=(Check-4-completion Check-5-all-done Check-8-file-existence Check-11-execution-evidence)
    ;;
esac

# The declared exclusions are ENFORCED for framework-proposal-v1 only. That
# profile has no passing users yet, so enforcing them there has zero blast
# radius and is the only way an audit-only mode can clear delivery-completion
# checks it declares do not apply. planning-maturity-v1 and
# delivery-completion-v1 are in live use and already pass with their list purely
# declarative; honouring it for them would newly ADMIT packets those checks
# currently refuse, so this helper is a no-op under every other profile.
check_is_applicable() {
  local check_id="$1"
  local declared

  [[ "$transition_audit_profile" == "framework-proposal-v1" ]] || return 0
  for declared in ${transition_not_applicable_checks[@]+"${transition_not_applicable_checks[@]}"}; do
    if [[ "$declared" == "$check_id" ]]; then
      return 1
    fi
  done
  return 0
}

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
  record_gate_ids_from_message fail "$message"
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
  record_gate_ids_from_message pass "$message"
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
workflow_grants_lint_script="$SCRIPT_DIR/workflow-runner-grants-lint.sh"

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

for scope_path in ${scope_files[@]+"${scope_files[@]}"}; do
  build_scope_analysis_units "$scope_path"
done

if [[ ${#scope_analysis_files[@]} -eq 0 ]]; then
  scope_analysis_files=(${scope_files[@]+"${scope_files[@]}"})
  for scope_path in ${scope_files[@]+"${scope_files[@]}"}; do
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
  for scope_path in ${scope_files[@]+"${scope_files[@]}"}; do
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
  for scope_path in ${scope_files[@]+"${scope_files[@]}"}; do
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

state_status="$transition_current_status"
state_workflow_mode="$transition_workflow_mode"
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
state_status_ceiling="$transition_target_status"
if [[ "$state_status" == "$state_status_ceiling" ]]; then
  pass "Workflow mode '$state_workflow_mode' permits current status '$state_status' (ceiling: $state_status_ceiling)"
elif [[ "$state_status_ceiling" == "done" ]]; then
  info "Workflow mode '$state_workflow_mode' allows status 'done'; current status is '$state_status'"
else
  info "Workflow mode '$state_workflow_mode' ceiling is '$state_status_ceiling'; current status is '$state_status'"
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
ceiling_forbids_code="$transition_source_edit_lockout_required"
ceiling_label="$transition_target_status"
g073_failures_before="$failures"

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
      deliverable_files_list="$(python3 -c '
import json, sys
try:
    d=json.load(open(sys.argv[1]))
    for f in (d.get("deliverableFiles") or []):
        if isinstance(f,str) and f.strip():
            print(f.strip())
except Exception:
    pass' "$state_file" 2>/dev/null || true)"
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
      if grep -qE "$source_code_pattern" <<< "$changed_file"; then
        if ! grep -qE "$allowed_path_pattern" <<< "$changed_file"; then
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
      if grep -qE "$source_code_pattern" <<< "$changed_file"; then
        if ! grep -qE "$allowed_path_pattern" <<< "$changed_file"; then
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
        if grep -qE "$source_code_pattern" <<< "$changed_file"; then
          if ! grep -qE "$allowed_path_pattern" <<< "$changed_file"; then
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
if [[ "$transition_source_edit_lockout_required" == "true" ]]; then
  if [[ "$failures" -gt "$g073_failures_before" ]]; then
    record_failed_gate G073
  else
    record_passed_gate G073
  fi
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
# CHECK 3G: Framework ownership/result contract integrity (G042/G063)
# =============================================================================
echo "--- Check 3G: Framework Ownership And Result Contract (G042/G063) ---"
if [[ -x "$framework_ownership_lint_script" || -f "$framework_ownership_lint_script" ]]; then
  _c3g_start=$(date +%s)
  _c3g_rc=0
  bubbles_run_with_timeout 30 bash "$framework_ownership_lint_script" >/tmp/bubbles-agent-ownership-lint.$$ 2>&1 || _c3g_rc=$?
  _c3g_elapsed=$(( $(date +%s) - _c3g_start ))
  if [[ "$_c3g_rc" -eq 124 ]]; then
    fail "Framework ownership lint TIMED OUT after 30s (BUG-001 guard) — G042/G063 not certified. Inspect $framework_ownership_lint_script for an unbounded walk."
  elif [[ "$_c3g_rc" -eq 0 ]]; then
    pass "Framework ownership lint passed — artifact ownership enforcement and concrete result contract are internally consistent (${_c3g_elapsed}s)"
  else
    fail "Framework ownership lint failed — G042/G063 cannot be certified during state transition"
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
  fail "Framework ownership lint script not found at $framework_ownership_lint_script — cannot enforce G042/G063"
fi
echo ""

# =============================================================================
# CHECK 3H: Workflow runner authorization (G064)
# =============================================================================
echo "--- Check 3H: Workflow Runner Authorization (G064) ---"
if [[ -x "$workflow_grants_lint_script" || -f "$workflow_grants_lint_script" ]]; then
  _c3h_rc=0
  bubbles_run_with_timeout 30 bash "$workflow_grants_lint_script" >/tmp/bubbles-workflow-grants-lint.$$ 2>&1 || _c3h_rc=$?
  if [[ "$_c3h_rc" -eq 124 ]]; then
    fail "Workflow runner grants lint TIMED OUT after 30s — G064 not certified"
  elif [[ "$_c3h_rc" -eq 0 ]]; then
    pass "Workflow runner grants lint passed — mode execution is top-level, direct, and authorized"
  else
    fail "Workflow runner grants lint failed — G064 cannot be certified during state transition"
    while IFS= read -r lint_line; do
      [[ -n "$lint_line" ]] || continue
      echo "   → $lint_line"
    done < /tmp/bubbles-workflow-grants-lint.$$
  fi
  rm -f /tmp/bubbles-workflow-grants-lint.$$
else
  fail "Workflow runner grants lint script not found at $workflow_grants_lint_script — cannot enforce G064"
fi
echo ""

# =============================================================================
# CHECK 3I: Assurance Certification Consistency (IMP-105 SCOPE-1)
# =============================================================================
echo "--- Check 3I: Assurance Certification Consistency ---"
assurance_cert_check_script="$SCRIPT_DIR/assurance-certification-check.sh"
if [[ -f "$assurance_cert_check_script" ]]; then
  _c3i_rc=0
  bubbles_run_with_timeout 30 bash "$assurance_cert_check_script" --feature-dir "$feature_dir" >/tmp/bubbles-assurance-cert-check.$$ 2>&1 || _c3i_rc=$?
  if [[ "$_c3i_rc" -eq 124 ]]; then
    fail "Assurance certification consistency check TIMED OUT after 30s — cannot certify the recorded assurance block"
  elif [[ "$_c3i_rc" -eq 0 ]]; then
    pass "Recorded certification.assurance block is internally consistent (or absent — backward-compatible no-op)"
  else
    fail "Recorded certification.assurance block is internally inconsistent — full has no gaps, fast must list independent-audit, prototype must be non-empty"
    while IFS= read -r lint_line; do
      [[ -n "$lint_line" ]] || continue
      echo "   → $lint_line"
    done < /tmp/bubbles-assurance-cert-check.$$
  fi
  rm -f /tmp/bubbles-assurance-cert-check.$$
else
  warn "Assurance certification consistency check script not found at $assurance_cert_check_script — skipping (advisory)"
fi
echo ""

# =============================================================================
# CHECK 4: Completion basis
#
# IMP-047 S-C (AC5, AC7). There is ONE completion authority per spec, never two.
#
# A checkbox proves that somebody ticked a box. What certification needs to know
# is whether the named scenario went RED and then GREEN on the SAME test with
# the SAME negative control — the only shape that distinguishes "the change
# worked" from "the test always passed". So when a spec carries a
# scenario-manifest.json, the required SCENARIO STATES are the basis and the
# checkbox count is NOT: the count is still reported, because an unchecked item
# is useful information, but it no longer decides certification.
#
# The legacy checkbox basis survives ONLY where the replacement cannot apply —
# a spec with no manifest. That is the migration rule from
# bubbles/registry/scenario-states.yaml, not an escape hatch: adding a manifest
# moves the spec onto the receipt basis permanently.
#
# Gate passes never advance a state (AC5): the resolver derives states from
# receipts carrying a scenarioBinding, and a passing gate carries none.
# =============================================================================
echo "--- Check 4: DoD Completion (Zero Unchecked) ---"
total_checked=0
total_unchecked=0
for scope_path in ${scope_files[@]+"${scope_files[@]}"}; do
  [[ -f "$scope_path" ]] || continue
  total_checked=$((total_checked + $(grep -cE '^\- \[x\] ' "$scope_path" || true)))
  total_unchecked=$((total_unchecked + $(grep -cE '^\- \[ \] ' "$scope_path" || true)))
done
total_dod=$((total_checked + total_unchecked))

info "DoD items total: $total_dod (checked: $total_checked, unchecked: $total_unchecked)"

scenario_basis="none"
if [[ -f "$feature_dir/scenario-manifest.json" && -f "$SCRIPT_DIR/scenario-state-resolve.sh" ]]; then
  # The basis applies only when the manifest declares a scenario the resolver
  # can KEY — a canonical `id`. A manifest whose scenarios carry no id cannot be
  # resolved at all, and refusing it would reject the spec on a schema
  # technicality rather than on evidence. That case keeps the legacy basis, the
  # same way a spec with no manifest does.
  if MANIFEST="$feature_dir/scenario-manifest.json" python3 -c '
import json, os, sys
try:
    data = json.load(open(os.environ["MANIFEST"], encoding="utf-8"))
except Exception:
    sys.exit(1)
rows = data if isinstance(data, list) else (data.get("scenarios") or [])
sys.exit(0 if any(isinstance(r, dict) and (r.get("id") or "").strip() for r in rows) else 1)
' 2>/dev/null; then
    scenario_basis="scenario-states"
  fi
fi

if [[ "$total_dod" -eq 0 ]]; then
  record_failed_check Check-4-structure
  fail "Resolved scope artifacts have ZERO DoD checkbox items — cannot verify completion"
elif [[ "$transition_audit_profile" == "planning-maturity-v1" ]]; then
  # A planning ceiling claims no delivery, so neither basis applies: there is no
  # completion to verify, by either accounting.
  info "NOT_APPLICABLE: Check-4-completion — planning maturity permits unchecked implementation DoD"
elif ! check_is_applicable Check-4-completion; then
  info "NOT_APPLICABLE: Check-4-completion — a framework proposal never implements, so it certifies no DoD completion"
elif [[ "$scenario_basis" == "scenario-states" ]]; then
  info "Completion basis: REQUIRED SCENARIO STATES (checkbox counts are reported, not decisive)"
  scenario_rc=0
  scenario_out="$(bash "$SCRIPT_DIR/scenario-state-resolve.sh" --spec-dir "$feature_dir" \
    --require RED_VERIFIED --require IMPLEMENTED --require GREEN_TARGETED \
    --require GREEN_LIVE --require REGRESSION_GREEN --require OBSERVED \
    --certifiable 2>&1)" || scenario_rc=$?
  if [[ "$scenario_rc" -eq 0 ]]; then
    pass "Every required scenario state is receipt-derived for every applicable scenario"
  elif [[ "$scenario_rc" -eq 2 ]]; then
    record_failed_check Check-4-scenario-states
    fail "Scenario-state resolution could not run for $feature_dir — completion cannot be verified"
    printf '%s\n' "$scenario_out" | while IFS= read -r _line; do echo "   → $_line"; done
  else
    record_failed_check Check-4-scenario-states
    fail "Required scenario states are NOT receipt-derived — certification refused"
    printf '%s\n' "$scenario_out" | while IFS= read -r _line; do echo "   → $_line"; done
  fi
  if [[ "$total_unchecked" -gt 0 ]]; then
    info "$total_unchecked unchecked DoD item(s) remain; reported for the operator, not counted as the completion basis"
  fi
elif [[ "$total_unchecked" -gt 0 ]]; then
  record_failed_check Check-4-completion
  fail "Resolved scope artifacts have $total_unchecked UNCHECKED DoD items — ALL must be [x] for 'done'"
  shown_unchecked=0
  for scope_path in ${scope_files[@]+"${scope_files[@]}"}; do
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
  pass "All $total_checked DoD items are checked [x] (legacy basis: no scenario-manifest.json)"
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
for scope_path in ${scope_files[@]+"${scope_files[@]}"}; do
  [[ -f "$scope_path" ]] || continue

  # BUG-026: consume the shared DoD parser (bubbles/scripts/dod-section-lib.sh).
  # A column-zero list item inside a DoD section that is NOT a checkbox is
  # format manipulation. The shared parser carries the correct tiered-DoD
  # boundary (nested #### tier subheadings are retained through depth 6 and the
  # section ends only at a same-or-shallower heading), so a valid tier no longer
  # terminates the section early (BUG026-F002). DoD-header matching stays
  # case-insensitive, matching the previous Check 4A behavior.
  while IFS=$'\t' read -r _c4a_rec _c4a_line _c4a_kind _c4a_text; do
    [[ "$_c4a_rec" == "LIST" && "$_c4a_kind" == "non-checkbox" ]] || continue
    fail "DoD format manipulation detected in ${scope_path#$feature_dir/} line $_c4a_line: ${_c4a_text:0:100}"
    fun_message format_bypass
    total_manipulated=$((total_manipulated + 1))
  done < <(dod_section_parse "$scope_path")
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
for scope_path in ${scope_files[@]+"${scope_files[@]}"}; do
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
for scope_path in ${scope_files[@]+"${scope_files[@]}"}; do
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
  record_failed_check Check-5-structure
  fail "Resolved scope artifacts have no scope status markers"
elif [[ "$transition_audit_profile" == "planning-maturity-v1" ]]; then
  info "NOT_APPLICABLE: Check-5-all-done — planning maturity permits canonical incomplete implementation scopes"
  for scope_path in ${scope_files[@]+"${scope_files[@]}"}; do
    [[ -f "$scope_path" ]] || continue
    if bubbles_status_lines "$scope_path" | grep -Eq '\*\*Status:\*\*.*Done' \
      && grep -Eq '^\- \[ \] ' "$scope_path"; then
      record_failed_check Check-5-status-honesty
      fail "Planning scope claims Done while unchecked DoD remain in ${scope_path#$feature_dir/} — false completion claim"
    fi
  done
elif ! check_is_applicable Check-5-all-done; then
  info "NOT_APPLICABLE: Check-5-all-done — a framework proposal terminates at a written proposal and certifies no scope delivery"
elif [[ "$not_started_scopes" -gt 0 ]]; then
  record_failed_check Check-5-all-done
  fail "Resolved scope artifacts have $not_started_scopes scope(s) still marked 'Not Started' — ALL scopes must be Done"
elif [[ "$in_progress_scopes" -gt 0 ]]; then
  record_failed_check Check-5-all-done
  fail "Resolved scope artifacts have $in_progress_scopes scope(s) still marked 'In Progress' — ALL scopes must be Done"
elif [[ "$blocked_scopes" -gt 0 ]]; then
  record_failed_check Check-5-all-done
  fail "Resolved scope artifacts have $blocked_scopes scope(s) still marked 'Blocked' — ALL scopes must be Done"
else
  pass "All $done_scopes scope(s) are marked Done"
fi

certification_scopes_block="$({
  grep -A40 '"certification"' "$state_file" 2>/dev/null \
    | awk '/"completedScopes"[[:space:]]*:/ {capture=1} capture {print} capture && /\]/ {exit}'
} || true)"

if [[ -n "$certification_scopes_block" ]]; then
  completed_scopes_body="$(printf '%s\n' "$certification_scopes_block" \
    | sed -E '1s/.*"completedScopes"[[:space:]]*:[[:space:]]*\[//' || true)"
else
  completed_scopes_body="$({
    awk '/"completedScopes"[[:space:]]*:/ {capture=1} capture {print} capture && /\]/ {exit}' "$state_file" \
      | sed -E '1s/.*"completedScopes"[[:space:]]*:[[:space:]]*\[//'
  } || true)"
fi

state_completed_scopes_count="$(printf '%s\n' "$completed_scopes_body" | grep -cE '"[^"]+"' || true)"

# BUG-011: an integer-ordinal array ([1,2,3,…]) matches no quoted string, so the
# count above reads 0 — the SAME value a genuinely empty array produces. The two
# states then report the identical "is EMPTY" message, which is what made a
# writer/reader type mismatch cost a schema comparison across four other specs to
# diagnose. Anything between the brackets that is not whitespace or a separator
# means the array is POPULATED, whatever its element type.
completed_scopes_payload="$(printf '%s\n' "$completed_scopes_body" \
  | sed -e '/\]/q' | sed -E 's/\].*$//' | tr -d '[:space:],')"

if [[ -n "$completed_scopes_payload" ]] && [[ "$state_completed_scopes_count" -eq 0 ]]; then
  fail "completedScopes is present but its entries are not string scope IDs (found: ${completed_scopes_payload:0:60}) — entries must be quoted scope IDs such as \"01-core-scope\", not ordinals; nothing can map an ordinal to a scope artifact"
elif [[ "$done_scopes" -gt 0 ]] && [[ "$state_completed_scopes_count" -eq 0 ]]; then
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
  for scope_path in ${scope_files[@]+"${scope_files[@]}"}; do
    [[ -f "$scope_path" ]] || continue
    scope_dir_name="$(basename "$(dirname "$scope_path")")"
    # Two directory conventions are in use: "NN-name" and "SCOPE-N-name".
    # Reading the ordinal with %%-* yields the literal "SCOPE" for the latter, so
    # no index row ever matched and parity silently degraded to a warning for
    # every scope in such a packet. Normalise the prefix away first.
    scope_dir_core="${scope_dir_name#[Ss][Cc][Oo][Pp][Ee]-}"
    # Strip leading "NN-" prefix to get the scope's natural-language identifier
    scope_dir_suffix="${scope_dir_core#[0-9]*-}"
    scope_dir_num="${scope_dir_core%%-*}"
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
    # Index tables commonly write the status as a markdown checkbox ("[x] Done")
    # while scope.md writes the bare word ("Done"). Comparing those raw reports a
    # fabrication for two spellings of the same status. Strip the checkbox marker
    # from both sides only — a real mismatch (Done vs In Progress) still fails.
    index_status_norm="$(echo "$index_status" | sed -E 's/^\[[ xX]\][[:space:]]*//')"
    scope_status_norm="$(echo "$scope_status_local" | sed -E 's/^\[[ xX]\][[:space:]]*//')"
    if [[ "$index_status_norm" != "$scope_status_norm" ]]; then
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
    for scope_path in ${scope_files[@]+"${scope_files[@]}"}; do
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

scope_declares_performance_contract() {
  local scope_path="$1"
  local performance_line=""
  local performance_signal='latency|throughput|p95|p99|response[ -]time|\bsla\b|\bslo\b'
  local affirmative_marker='target|budget|threshold|objective|guarantee|percentile|no more than|at most|at least|less than|greater than|under[[:space:]]+[0-9]|within[[:space:]]+[0-9]|[0-9]+([.][0-9]+)?[[:space:]]*(ms|milliseconds?|seconds?|rps|requests?[[:space:]]+per[[:space:]]+second|%|percent(age)?|ops)'
  local quantitative_marker='no more than|at most|at least|less than|greater than|under[[:space:]]+[0-9]|within[[:space:]]+[0-9]|[0-9]+([.][0-9]+)?[[:space:]]*(ms|milliseconds?|seconds?|rps|requests?[[:space:]]+per[[:space:]]+second|%|percent(age)?|ops)'
  local opt_out_marker='observability[^.;]*(opted out|disabled|unavailable|not applicable)|\bno[[:space:]]+(trace[[:space:]]+or[[:space:]]+)?(sla|slo)\b|\b(sla|slo|latency|throughput|p95|p99|response[ -]time)[^.;]*(not applicable|opted out|disabled|unavailable|absent|not declared|not required)|does not declare[^.;]*(sla|slo|latency|throughput|p95|p99|response[ -]time)|\bno[^.;]*(sla|slo|latency|throughput|p95|p99|response[ -]time)[^.;]*(evidence|target|budget|threshold|objective|guarantee)?[^.;]*(injected|captured|declared|required|available)?'

  while IFS= read -r performance_line || [[ -n "$performance_line" ]]; do
    if ! grep -Eiq "$performance_signal" <<< "$performance_line"; then
      continue
    fi

    if grep -Eiq "$opt_out_marker" <<< "$performance_line"; then
      # A concrete threshold wins over broad negation: "no more than 200 ms"
      # is an upper bound, while "no SLO target is declared" is an opt-out.
      if grep -Eiq "$quantitative_marker" <<< "$performance_line"; then
        return 0
      fi
      continue
    fi

    if grep -Eiq "$affirmative_marker" <<< "$performance_line"; then
      return 0
    fi

    return 0
  done < "$scope_path"

  return 1
}

for scope_path in ${scope_files[@]+"${scope_files[@]}"}; do
  [[ -f "$scope_path" ]] || continue

  # BUG-032: mention is not affirmation. Explicit no-SLA/no-SLO, not-applicable,
  # unavailable, and opted-out lines are ignored unless the same line carries
  # a target, budget, threshold, guarantee, comparator, or quantitative unit.
  if scope_declares_performance_contract "$scope_path"; then
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

# MERGE, never short-circuit. A truthy `certifiedCompletedPhases` used to win
# outright via `or`, so a spec carrying certification ["validate"] alongside 14
# execution claims reported every other phase as unrecorded — while Check 6B,
# reading completedPhaseClaims directly, passed those same entries. One run then
# asserted both "phase not recorded" and "that phase's record has valid
# provenance". Concatenating is safe: _phase_name() below normalizes bare
# strings and dict claim records alike, and dict.fromkeys dedups the result.
selected_phases = list(certification_phases) + list(execution_phase_claims) + list(legacy_phases)

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
  # Read the canonical mode -> required-specialist mapping. This table was
  # duplicated here as a case statement that required-specialists.yaml mirrored,
  # with a shadow comparator holding the two in step; the registry is now the
  # only copy. yq is the normal path, and the awk fallback parses the flow-style
  # lists so Check 6 still resolves under a minimal PATH. An unresolved mode
  # leaves the array empty and falls through to the derivation below, which is
  # the same fail-closed path an unlisted mode always took.
  _rs_registry="$SCRIPT_DIR/../registry/required-specialists.yaml"
  if [[ -f "$_rs_registry" ]]; then
    if command -v yq >/dev/null 2>&1; then
      # strenv, not --arg: the pinned yq is Go yq, which has no --arg flag and
      # would return empty for every mode. It also keeps the mode out of the
      # expression text.
      _rs_list="$(_rs_mode="$state_workflow_mode" yq -r '.modes[strenv(_rs_mode)] // [] | join(" ")' "$_rs_registry" 2>/dev/null || true)"
    else
      _rs_list="$(awk -v want="$state_workflow_mode" '
        /^modes:[[:space:]]*$/ { in_modes = 1; next }
        /^[a-zA-Z]/ { in_modes = 0 }
        in_modes == 1 && $0 ~ /^[[:space:]]+[a-z0-9][a-z0-9-]*:[[:space:]]*\[/ {
          key = $0
          sub(/^[[:space:]]*/, "", key)
          sub(/:.*$/, "", key)
          if (key != want) next
          val = $0
          sub(/^[^[]*\[/, "", val)
          sub(/\].*$/, "", val)
          gsub(/,/, " ", val)
          gsub(/"/, "", val)
          gsub(/[[:space:]]+/, " ", val)
          sub(/^ /, "", val)
          sub(/ $/, "", val)
          print val
        }
      ' "$_rs_registry" 2>/dev/null || true)"
    fi
    for _rs_phase in $_rs_list; do
      required_specialists+=("$_rs_phase")
    done
  else
    warn "Check 6: required-specialists registry missing at $_rs_registry — falling back to phaseOrder derivation"
  fi

  # IMP-105-SCOPE-3-FALLBACK-BEGIN
  # IMP-105 SCOPE-3 — close the Check 6 fail-open hole. A mode ABSENT from the
  # explicit case above left required_specialists empty, so Check 6 imposed ZERO
  # specialist-completion enforcement (the historical rapid-tool-delivery bug).
  # For any unlisted mode, DERIVE a safe non-empty fallback: the intersection of
  # the mode's declared phaseOrder with the canonical delivery-specialist phase
  # set. Control/planning/conditional phases (select finalize discover analyze
  # bootstrap interrogate releases devops redteam bug review journey) are excluded
  # by that intersection (they are simply not in the core set). TWO further guards
  # keep the fallback from over-requiring modes whose phaseOrder is NOT a
  # parent-execution contract (empirically verified against the whole mode
  # registry — a mini shadow-compare of the UNLISTED set the fallback governs):
  #   (a) PROFILE — derive ONLY for delivery-completion profiles. planning-maturity
  #       modes (product-to-planning, spec-scope-hardening) declare a full delivery
  #       phaseOrder as the PLAN of future work, not phases the parent executes at
  #       planning maturity, so they are excluded. (Modes with no transitionAudit
  #       profile never reach Check 6 — transition-contract-resolver.sh blocks them
  #       first with E009-AUDIT-PROFILE-MISSING/UNSUPPORTED.)
  #   (b) DISPATCHER — skip FAN-OUT / top-level-runtime modes
  #       (requiresTopLevelRuntime: true — autonomous-goal, autonomous-sprint,
  #       idea-to-release-completion, retro-quality-sweep): their phaseOrder is a
  #       DISPATCH plan; the specialist phases run in the child workflows they
  #       dispatch, exactly why the explicit table pins the listed dispatchers
  #       iterate / stochastic-quality-sweep to a minimal {validate,audit} set.
  # After both guards, the only unlisted modes that DERIVE are genuine delivery
  # modes omitted from the table by oversight (devops-to-doc, redteam-to-doc,
  # retro-to-harden, retro-to-simplify, delivery-lockdown) — precisely the fail-
  # open hole this scope closes. The explicit per-mode table stays authoritative
  # for listed modes; full table replacement with per-mode shadow-compare is
  # IMP-105 SCOPE-7's job, not this scope's.
  if [[ ${#required_specialists[@]} -eq 0 && -n "$state_workflow_mode" && ( "$transition_audit_profile" == "delivery-completion-v1" || "$transition_audit_profile" == "delivery-completion-fast-v1" ) ]]; then
    _imp105_resolved="$(BUBBLES_MODE_GRANDFATHER=1 bubbles_run_with_timeout 30 bash "$SCRIPT_DIR/mode-resolver.sh" "$state_workflow_mode" 2>/dev/null || true)"
    if [[ -z "$_imp105_resolved" ]]; then
      warn "Check 6: mode '$state_workflow_mode' absent from explicit table AND its definition is unresolvable — cannot derive a specialist requirement (advisory)"
    elif [[ "$(yq -r '.constraints.requiresTopLevelRuntime // false' <<< "$_imp105_resolved" 2>/dev/null || echo false)" == "true" ]]; then
      info "Check 6: mode '$state_workflow_mode' is a top-level-runtime dispatcher (requiresTopLevelRuntime) — specialist completion is delegated to the child workflows it dispatches; its phaseOrder is a dispatch plan, NOT a parent requirement, so it is NOT derived (IMP-105 SCOPE-3 avoids dispatcher over-require)"
    else
      _imp105_phase_order="$(yq -r '.phaseOrder[]' <<< "$_imp105_resolved" 2>/dev/null || true)"
      if [[ -n "$_imp105_phase_order" ]]; then
        _imp105_core="implement test regression simplify gaps harden stabilize security validate audit chaos docs"
        while IFS= read -r _imp105_ph; do
          [[ -n "$_imp105_ph" ]] || continue
          for _imp105_c in $_imp105_core; do
            if [[ "$_imp105_ph" == "$_imp105_c" ]]; then
              required_specialists+=("$_imp105_ph")
              break
            fi
          done
        done <<< "$_imp105_phase_order"
        if [[ ${#required_specialists[@]} -gt 0 ]]; then
          info "Check 6: mode '$state_workflow_mode' absent from explicit table — derived ${#required_specialists[@]} required specialist phase(s) from its phaseOrder (IMP-105 SCOPE-3 fallback closes the fail-open default)"
        fi
      else
        warn "Check 6: mode '$state_workflow_mode' absent from explicit table AND its phaseOrder is empty/unresolvable — cannot derive a specialist requirement (advisory)"
      fi
    fi
  fi
  # IMP-105-SCOPE-3-FALLBACK-END

  if [[ ${#required_specialists[@]} -gt 0 ]]; then
    missing_phases=0
    for specialist_phase in "${required_specialists[@]}"; do
      if grep -qE "\"$specialist_phase\"" <<< "$state_completed_phases_block"; then
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
      python3 -c 'import json, sys; data=json.load(open(sys.argv[1])); execution=(data.get("execution") or {}); history=(execution.get("executionHistory") or data.get("executionHistory") or []); print("\n".join((entry.get("agent") or "") for entry in history if isinstance(entry, dict) and entry.get("agent")))' "$state_file"
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
    python3 -c '
import json, sys, os
spec_dir = os.path.dirname(sys.argv[1])
with open(sys.argv[1]) as f:
    data = json.load(f)
history = data.get("execution", {}).get("executionHistory", data.get("executionHistory", []))
for entry in history:
    agent = entry.get("agent", "")
    phases = entry.get("phasesExecuted", [])
    provenance = entry.get("provenanceMode", "specialist")
    expanded_by = entry.get("expandedBy", "")
    reason = (entry.get("expansionReason", "") or "").replace("\t", " ").replace("\n", " ")
    ev_ref = (entry.get("expansionEvidenceRef", "") or "").replace("\t", " ")
    for p in phases:
        print(f"{agent}\t{p}\t{provenance}\t{expanded_by}\t{reason}\t{ev_ref}")
' "$state_file" 2>/dev/null
  } || true)"

  claimed_phases="$({
    python3 -c '
import json, sys
with open(sys.argv[1]) as f:
    data = json.load(f)
claims = data.get("execution", {}).get("completedPhaseClaims", [])
certified = data.get("certification", {}).get("certifiedCompletedPhases", [])
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
names = []
for entry in list(claims) + list(certified):
    resolved = _phase_name(entry)
    if resolved is not None:
        names.append(resolved)
for p in set(names):
    print(p)
' "$state_file" 2>/dev/null
  } || true)"

  if [[ -n "$claimed_phases" ]]; then
    # Orchestrator allowlist for parent-expansion (sourced from workflows.yaml is future work;
    # for now hardcode the three registered orchestrators).
    orchestrator_allowlist="bubbles.workflow bubbles.goal bubbles.sprint bubbles.iterate"
    # Legacy read compatibility: historical v6/v7 state may contain parent-expanded
    # phase provenance. New executions use direct-authorized-runner (Gate G064).
    expansion_reason_regex='runSubagent|tool unavailable|nested runtime|capability missing|parent-expand|nested workflow'
    spec_dir_for_evidence="$(dirname "$state_file")"

    # BUG-020: the set of agents that may execute a phase owned by
    # `activeWorkflowRunner`. agent-capabilities.yaml `workflowModeGrants.agents`
    # is the same list Gate G064 lints, so it is the only copy. yq is the normal
    # path; without it the resolution degrades to the parent-expansion allowlist
    # above rather than introducing a new hard dependency.
    workflow_runner_agents=""
    capabilities_registry_file=""
    for _pa_candidate in \
      "$guard_repo_root/bubbles/agent-capabilities.yaml" \
      "$guard_repo_root/.github/bubbles/agent-capabilities.yaml" \
      "$script_repo_root/bubbles/agent-capabilities.yaml" \
      "$script_repo_root/.github/bubbles/agent-capabilities.yaml"; do
      if [[ -f "$_pa_candidate" ]]; then
        capabilities_registry_file="$_pa_candidate"
        break
      fi
    done
    if [[ -n "$capabilities_registry_file" ]] && command -v yq >/dev/null 2>&1; then
      workflow_runner_agents="$(yq -r '.workflowModeGrants.agents // {} | keys | join(" ")' "$capabilities_registry_file" 2>/dev/null || true)"
    fi
    [[ -n "$workflow_runner_agents" ]] || workflow_runner_agents="$orchestrator_allowlist"

    agent_definition_exists() {
      local agent="$1"
      local definition="${agent}.agent.md"
      local candidate

      for candidate in \
        "$guard_repo_root/agents/$definition" \
        "$guard_repo_root/.github/agents/$definition" \
        "$script_repo_root/agents/$definition" \
        "$script_repo_root/.github/agents/$definition"; do
        [[ -f "$candidate" ]] && return 0
      done
      return 1
    }

    # Phase -> owning-agent resolution. workflows.yaml `phases.<phase>.owner`
    # is the authority. An absent phase or a declared owner with no agent
    # definition is a framework-integrity refusal, never a cue to synthesize
    # `bubbles.<phase>`. Existing legacy identities remain accepted only when
    # that identity names a real shipped agent definition.
    resolve_phase_owner() {
      local phase="$1"
      local declared=""
      local legacy=""
      local phase_missing=""

      phase_owner_resolution=""
      phase_owner_agents=""
      phase_owner_detail=""

      if [[ -z "$workflow_registry_file" ]]; then
        phase_owner_resolution="registry-unavailable"
        phase_owner_detail="no workflows.yaml registry could be resolved"
        return 0
      fi
      if ! command -v yq >/dev/null 2>&1; then
        phase_owner_resolution="registry-unavailable"
        phase_owner_detail="yq is unavailable for reading $workflow_registry_file"
        return 0
      fi

      if ! phase_missing="$(_pa_phase="$phase" yq -r '.phases[strenv(_pa_phase)] == null' "$workflow_registry_file" 2>/dev/null)"; then
        phase_owner_resolution="registry-unavailable"
        phase_owner_detail="could not query $workflow_registry_file"
        return 0
      fi
      if [[ "$phase_missing" == "true" ]]; then
        phase_owner_resolution="unregistered-phase"
        phase_owner_detail="$workflow_registry_file"
        return 0
      fi
      if [[ "$phase_missing" != "false" ]]; then
        phase_owner_resolution="registry-unavailable"
        phase_owner_detail="unexpected phase lookup result '$phase_missing' from $workflow_registry_file"
        return 0
      fi

      if ! declared="$(_pa_phase="$phase" yq -r '.phases[strenv(_pa_phase)].owner // ""' "$workflow_registry_file" 2>/dev/null)"; then
        phase_owner_resolution="registry-unavailable"
        phase_owner_detail="could not read owner for phase '$phase' from $workflow_registry_file"
        return 0
      fi
      if [[ -z "$declared" ]]; then
        phase_owner_resolution="owner-missing"
        phase_owner_detail="$workflow_registry_file"
        return 0
      fi

      case "$phase" in
        analyze) legacy='bubbles.analyst' ;;
        *) legacy="bubbles.$phase" ;;
      esac

      case "$declared" in
        activeWorkflowRunner)
          phase_owner_agents="$workflow_runner_agents"
          ;;
        *)
          if ! agent_definition_exists "$declared"; then
            phase_owner_resolution="owner-unregistered"
            phase_owner_detail="$declared"
            return 0
          fi
          phase_owner_agents="$declared"
          ;;
      esac

      if [[ "$legacy" != "$declared" ]] && agent_definition_exists "$legacy"; then
        phase_owner_agents="$phase_owner_agents $legacy"
      fi
      phase_owner_agents="${phase_owner_agents# }"
      phase_owner_resolution="registered"
    }

    provenance_failures=0
    phase_registry_failures=0
    while IFS= read -r claimed_phase; do
        [[ -z "$claimed_phase" ]] && continue
        resolve_phase_owner "$claimed_phase"
        case "$phase_owner_resolution" in
          registered)
            expected_agent="$phase_owner_agents"
            ;;
          unregistered-phase)
            fail "Phase '$claimed_phase' is not registered in phase registry '$phase_owner_detail' (Gate G022 framework integrity); refusing without synthesizing a phantom owner"
            phase_registry_failures=$((phase_registry_failures + 1))
            continue
            ;;
          owner-missing)
            fail "Phase '$claimed_phase' is registered in '$phase_owner_detail' but declares no owner (Gate G022 framework integrity)"
            phase_registry_failures=$((phase_registry_failures + 1))
            continue
            ;;
          owner-unregistered)
            fail "Phase '$claimed_phase' declares owner '$phase_owner_detail', but that owner has no shipped agent definition (Gate G022 framework integrity)"
            phase_registry_failures=$((phase_registry_failures + 1))
            continue
            ;;
          *)
            fail "Phase '$claimed_phase' owner could not be resolved: $phase_owner_detail (Gate G022 framework integrity)"
            phase_registry_failures=$((phase_registry_failures + 1))
            continue
            ;;
        esac
        matched="false"

        # Pass 1: specialist provenance (existing behavior). BUG-020: the owner
        # is now a SET (a registry-declared owner plus the legacy convention, or
        # every granted workflow runner for an `activeWorkflowRunner` phase), so
        # awk reports which member actually supplied the provenance.
        matched_agent="$(echo "$execution_history_block" | awk -F'\t' -v alist="$expected_agent" -v p="$claimed_phase" '
          BEGIN { owner_count = split(alist, owners, " ") }
          $2==p && ($3=="" || $3=="specialist") {
            for (i = 1; i <= owner_count; i++) {
              if ($1 == owners[i]) { print $1; exit }
            }
          }')"
        if [[ -n "$matched_agent" ]]; then
          pass "Phase '$claimed_phase' has specialist provenance from $matched_agent"
          matched="true"
        # bubbles.bug delegation shortcut for implement/test
        elif [[ "$claimed_phase" == "implement" || "$claimed_phase" == "test" ]] && echo "$execution_history_block" | awk -F'\t' -v p="$claimed_phase" '$1=="bubbles.bug" && $2==p && ($3=="" || $3=="specialist") {found=1} END{exit !found}'; then
          pass "Phase '$claimed_phase' has delegated provenance from bubbles.bug"
          matched="true"
        fi

        # Pass 2: legacy parent-expanded provenance (read-only compatibility)
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
            elif ! grep -qiE "$expansion_reason_regex" <<< "$pe_reason"; then
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
                parent_expanded_phases=$((parent_expanded_phases + 1))
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
    if [[ "$phase_registry_failures" -gt 0 ]]; then
      fail "$phase_registry_failures phase claim(s) violate the registered phase-owner contract — framework integrity check failed"
    fi
  elif [[ -n "$execution_history_block" ]]; then
    info "No phase claims to verify provenance for"
  else
    info "No phase claims or executionHistory found — phase provenance check not applicable"
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
# For (b) and (c) an entry may declare its span weak rather than have it read as
# fabricated: durationUnmeasured/durationUnmeasuredReason for a span that was
# never measured, timestampReconstructed/timestampReconstructedReason for one
# recovered after the fact. Both demand a substantive reason and both are still
# reported, so neither is a silent bypass.
echo "--- Check 7A: executionHistory Timestamp Plausibility ---"
exec_history_analysis="$(python3 - "$state_file" <<'PY'
import json
import sys
from datetime import datetime

ZERO_DURATION_EXEMPT = {"finalize", "select"}
# One threshold for every "I am declaring this record weak" escape on this
# surface. Three literals drift; one does not.
DECLARED_REASON_MIN = 20

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

def uniform_interval(instants):
    """Constant positive spacing over >=3 instants — the fabrication signature.

    Shared by BOTH timestamp surfaces this check analyses (executionHistory
    spans and completedPhaseClaims[].claimedAt). A second copy of this rule is
    exactly how the two G068 implementations drifted apart in BUG-004.
    """
    if len(instants) < 3:
        return None
    gaps = [int((instants[i] - instants[i - 1]).total_seconds())
            for i in range(1, len(instants))]
    if len(set(gaps)) == 1 and gaps[0] > 0:
        return gaps[0]
    return None

def first_backwards_step(labelled):
    """First point where a recorded sequence of instants runs backwards."""
    for i in range(1, len(labelled)):
        if labelled[i][1] < labelled[i - 1][1]:
            return (f"{labelled[i - 1][0]}@{labelled[i - 1][1].isoformat()}"
                    f" -> {labelled[i][0]}@{labelled[i][1].isoformat()}")
    return None

try:
    with open(sys.argv[1], encoding="utf-8") as fh:
        data = json.load(fh)
except Exception:
    sys.exit(0)

history = []
# executionHistory is written at the TOP level by most agents and under
# execution.* by others. Selecting only one location silently yields [] for the
# other, which turned this whole check into a no-op that reported "fewer than 3
# entries" against a packet holding fourteen. Check 6B already falls back this
# way; if this check does not, the two disagree about the same array.
execution_obj = data.get("execution") if isinstance(data.get("execution"), dict) else {}
raw = execution_obj.get("executionHistory")
if not isinstance(raw, list):
    raw = data.get("executionHistory")
if not isinstance(raw, list):
    raw = []

entries = []
for entry in raw:
    if not isinstance(entry, dict):
        continue
    # Entry timestamps are startedAt plus completedAt or finishedAt.
    # runStartedAt is an EXECUTION-level field, not an entry field: across every
    # recorded packet it appears zero times on an entry, so reading it here made
    # the loop `continue` on all of them. This check had never once evaluated an
    # entry, which is why it did not catch a set of fabricated timestamps.
    started = parse_ts(entry.get("startedAt") or entry.get("runStartedAt"))
    completed = parse_ts(
        entry.get("completedAt")
        or entry.get("finishedAt")
        or entry.get("runCompletedAt")
    )
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
        # An entry may DECLARE that its span was never measured — the writer
        # stored one instant rather than a start and a finish. That is a
        # different condition from a fabricated span, and conflating them makes
        # the zero-duration signal unusable on any record written before a span
        # was required. The declaration must carry a substantive reason, so it
        # cannot be used as a silent exemption, and it is reported either way.
        "unmeasured": entry.get("durationUnmeasured") is True,
        "unmeasured_reason": (entry.get("durationUnmeasuredReason") or "").strip()
            if isinstance(entry.get("durationUnmeasuredReason"), str) else "",
        # An entry may separately DECLARE that its span was reconstructed after
        # the fact rather than measured from a clock. An overlap touching such a
        # span is evidence that the RECORD is imprecise, not that two agents ran
        # concurrently, and those are different findings. Without this the only
        # ways to clear a historical overlap are to invent replacement
        # timestamps — the fabrication this check exists to catch — or to leave
        # the packet permanently uncertifiable. Same cost as the declaration
        # above: the reason must be substantive, and it is reported either way.
        "reconstructed": entry.get("timestampReconstructed") is True,
        "reconstructed_reason": (entry.get("timestampReconstructedReason") or "").strip()
            if isinstance(entry.get("timestampReconstructedReason"), str) else "",
    })

# --- Second input set: completedPhaseClaims[].claimedAt (BUG-013) ------------
# executionHistory is the narrative log; completedPhaseClaims is the load-bearing
# assertion Gates G022/G027 read to decide whether a phase was performed. This
# analysis only ever ran over the log, so ten claims sitting on an exact
# 600-second grid — the precise signature `uniform_interval` recognises — were
# read by no check at all. Same helpers, second input set: not a second copy.
# Emitted BEFORE the history early-exit so a packet with a short history still
# gets its claims analysed.
claims_raw = execution_obj.get("completedPhaseClaims")
if not isinstance(claims_raw, list):
    claims_raw = data.get("completedPhaseClaims")
if not isinstance(claims_raw, list):
    # Absence is not evidence of fabrication — abstain, as Check 7C does for an
    # absent executionHistory.
    print("CLAIMS_ABSENT=1")
else:
    claim_instants = []
    claim_unreconciled = []
    for claim in claims_raw:
        if not isinstance(claim, dict):
            continue
        claimed = parse_ts(claim.get("claimedAt"))
        if claimed is None:
            continue
        label = claim.get("phase") or claim.get("agent") or "<unknown>"
        reason = claim.get("claimedAtUnreconciledReason")
        reason = reason.strip() if isinstance(reason, str) else ""
        # The claim-side equivalent of the durationUnmeasured escape: a claim
        # whose instant could never be reconciled against a real run may DECLARE
        # that, and is then excluded from the grid rather than blocked. Same cost
        # as the other declarations — an absent or perfunctory reason buys
        # nothing, or the escape is a bypass with extra steps.
        if claim.get("claimedAtUnreconciled") is True and len(reason) >= DECLARED_REASON_MIN:
            claim_unreconciled.append(label)
            continue
        claim_instants.append((label, claimed))
    if claim_unreconciled:
        print(f"CLAIM_UNRECONCILED={'|'.join(claim_unreconciled)}")
    print(f"CLAIM_COUNT={len(claim_instants)}")
    claim_disorder = first_backwards_step(claim_instants)
    if claim_disorder:
        print(f"CLAIM_OUT_OF_ORDER={claim_disorder}")
    claim_interval = uniform_interval([instant for _, instant in claim_instants])
    if claim_interval is not None:
        print(f"CLAIM_UNIFORM_INTERVAL={claim_interval}")

if len(entries) < 3:
    print(f"COUNT={len(entries)}")
    sys.exit(0)

entries.sort(key=lambda e: e["started"])
print(f"COUNT={len(entries)}")

# Check uniform intervals between consecutive runStartedAt timestamps
history_interval = uniform_interval([e["started"] for e in entries])
if history_interval is not None:
    print(f"UNIFORM_INTERVAL={history_interval}")

# Check zero-duration entries (excluding intentionally zero phases)
zero_dur_offenders = []
unmeasured_spans = []
for e in entries:
    duration = (e["completed"] - e["started"]).total_seconds()
    if duration <= 0:
        if not e["phases"] or any(p not in ZERO_DURATION_EXEMPT for p in e["phases"]):
            label = f"{e['agent']}:{','.join(e['phases']) or '?'}"
            # A declared-unmeasured span is surfaced, not blocked. An EMPTY or
            # perfunctory reason is still an offender: the declaration has to
            # cost something or it is just a bypass with extra steps.
            if e["unmeasured"] and len(e["unmeasured_reason"]) >= DECLARED_REASON_MIN:
                unmeasured_spans.append(label)
            else:
                zero_dur_offenders.append(label)
if unmeasured_spans:
    print(f"UNMEASURED_SPANS={'|'.join(unmeasured_spans)}")
if zero_dur_offenders:
    print(f"ZERO_DURATION={'|'.join(zero_dur_offenders)}")

# Check overlapping entries (entry N+1 starts before entry N ends)
overlaps = []
reconstructed_overlaps = []
for i in range(1, len(entries)):
    prev = entries[i-1]
    curr = entries[i]
    if curr["started"] < prev["completed"]:
        detail = (
            f"{prev['agent']}({prev['started'].isoformat()}-{prev['completed'].isoformat()}) "
            f"overlaps {curr['agent']}({curr['started'].isoformat()})"
        )
        # ONE declared side is enough. If either span was reconstructed rather
        # than measured, the pair cannot testify to concurrency at all, so
        # demanding the declaration on both would force a false declaration
        # onto the entry whose timestamps are actually trustworthy.
        declared = [
            e["agent"] for e in (prev, curr)
            if e["reconstructed"]
            and len(e["reconstructed_reason"]) >= DECLARED_REASON_MIN
        ]
        if declared:
            reconstructed_overlaps.append(f"{detail} [reconstructed: {','.join(declared)}]")
        else:
            overlaps.append(detail)
if reconstructed_overlaps:
    print(f"RECONSTRUCTED_OVERLAPS={len(reconstructed_overlaps)}")
    for line in reconstructed_overlaps:
        print(f"RECONSTRUCTED_OVERLAP_DETAIL={line}")
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

  # Surfaced, never silent: a declared-unmeasured span is a weaker record than a
  # measured one, and a reader should be told which entries carry that weakness.
  unmeasured_line="$(echo "$exec_history_analysis" | grep -E '^UNMEASURED_SPANS=' | head -n 1 | sed 's/^UNMEASURED_SPANS=//' || true)"
  if [[ -n "$unmeasured_line" ]]; then
    info "executionHistory declares unmeasured spans (single instant recorded, reason given): $unmeasured_line"
  fi

  zero_dur_line="$(echo "$exec_history_analysis" | grep -E '^ZERO_DURATION=' | head -n 1 | sed 's/^ZERO_DURATION=//' || true)"
  if [[ -n "$zero_dur_line" ]]; then
    fail "executionHistory contains zero-duration entries for non-trivial phases: $zero_dur_line"
  fi

  # Surfaced, never silent: a declared-reconstructed overlap is still a weaker
  # record than a measured one, so the reader is told which spans carry it and
  # why the pair was not blocked.
  reconstructed_overlap_count="$(echo "$exec_history_analysis" | grep -E '^RECONSTRUCTED_OVERLAPS=' | head -n 1 | sed 's/^RECONSTRUCTED_OVERLAPS=//' || true)"
  if [[ -n "$reconstructed_overlap_count" ]] && [[ "$reconstructed_overlap_count" -gt 0 ]]; then
    info "executionHistory has $reconstructed_overlap_count overlapping entries whose spans are DECLARED reconstructed (reason given) — recorded as an imprecise record, not concurrent execution"
    while IFS= read -r detail; do
      info "$detail"
    done < <(echo "$exec_history_analysis" | grep -E '^RECONSTRUCTED_OVERLAP_DETAIL=' | sed 's/^RECONSTRUCTED_OVERLAP_DETAIL=//')
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

# BUG-013: the SAME uniform-interval and ordering analysis over the second
# timestamp surface. It is reported outside the executionHistory branch above
# because a packet with a short history still carries load-bearing phase claims.
claim_unreconciled_line="$(echo "$exec_history_analysis" | grep -E '^CLAIM_UNRECONCILED=' | head -n 1 | sed 's/^CLAIM_UNRECONCILED=//' || true)"
if [[ -n "$claim_unreconciled_line" ]]; then
  info "completedPhaseClaims declares unreconciled claim timestamps (reason given): $claim_unreconciled_line"
fi

claim_count="$(echo "$exec_history_analysis" | grep -E '^CLAIM_COUNT=' | head -n 1 | sed 's/^CLAIM_COUNT=//' || true)"
if echo "$exec_history_analysis" | grep -qE '^CLAIMS_ABSENT=1$'; then
  info "No completedPhaseClaims recorded — claim-timestamp plausibility abstains (an absent record is not evidence of fabrication)"
elif [[ -z "$claim_count" ]] || [[ "$claim_count" -lt 3 ]]; then
  info "completedPhaseClaims has fewer than 3 reconcilable claimedAt values — claim-timestamp plausibility skipped"
else
  info "completedPhaseClaims claimedAt values analyzed: $claim_count"

  claim_uniform_interval="$(echo "$exec_history_analysis" | grep -E '^CLAIM_UNIFORM_INTERVAL=' | head -n 1 | sed 's/^CLAIM_UNIFORM_INTERVAL=//' || true)"
  if [[ -n "$claim_uniform_interval" ]]; then
    fail "completedPhaseClaims has $claim_count claimedAt values with identical ${claim_uniform_interval}s intervals — FABRICATION INDICATOR"
  fi

  claim_disorder_line="$(echo "$exec_history_analysis" | grep -E '^CLAIM_OUT_OF_ORDER=' | head -n 1 | sed 's/^CLAIM_OUT_OF_ORDER=//' || true)"
  if [[ -n "$claim_disorder_line" ]]; then
    fail "completedPhaseClaims claimedAt runs backwards: $claim_disorder_line — a phase cannot be claimed complete before the phase recorded ahead of it"
  fi

  if [[ -z "$claim_uniform_interval" ]] && [[ -z "$claim_disorder_line" ]]; then
    pass "completedPhaseClaims claimedAt timestamps look plausible (no uniform spacing, no backwards ordering)"
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

# Same top-level / execution.* fallback as Check 7A. Without it this counted zero
# implement runs on a packet that records one, then "passed" by agreeing with its
# own empty read.
execution_obj = data.get("execution") if isinstance(data.get("execution"), dict) else {}
history = execution_obj.get("executionHistory")
if not isinstance(history, list):
    history = data.get("executionHistory")
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
# CHECK 7C: Phase-Claim Execution Backing
#
# Check 7A analyses executionHistory only, so a completedPhaseClaims entry with
# no history entry behind it is structurally invisible to it — a phase can be
# claimed complete with no record that it ever ran. Two severities, because the
# two shapes are not equally damning:
#   ZERO-BACKING (block) — a phase is claimed and executionHistory holds no entry
#     for it at all. There is no reading of that which is merely untidy.
#   EXCESS (warn) — more claims for a phase than recorded runs of it. Usually a
#     run that forgot its history entry, but incremental claiming from a single
#     run is a legitimate pattern, so this is surfaced rather than blocked.
# =============================================================================
echo "--- Check 7C: Phase-Claim Execution Backing ---"
claim_backing_analysis="$(python3 - "$state_file" <<'PY'
import json
import sys

try:
    with open(sys.argv[1], encoding="utf-8") as fh:
        data = json.load(fh)
except Exception:
    sys.exit(0)

execution = data.get("execution") if isinstance(data.get("execution"), dict) else {}
claims = execution.get("completedPhaseClaims")
if not isinstance(claims, list):
    claims = []

# Same top-level / execution.* fallback the other executionHistory readers use.
history = execution.get("executionHistory")
if not isinstance(history, list):
    history = data.get("executionHistory")
if not isinstance(history, list):
    history = []

claimed = {}
for claim in claims:
    # completedPhaseClaims is written BOTH ways in the wild: a list of plain
    # phase-name strings, and a list of {"phase": ...} objects. Accepting only
    # the dict shape made this gate inert against the string shape — every
    # element was skipped, `claimed` stayed empty, and an anti-fabrication check
    # reported NO_CLAIMS and passed. Check 6B's _phase_name already normalises
    # both; this now matches it.
    phase = None
    if isinstance(claim, str):
        phase = claim
    elif isinstance(claim, dict):
        candidate = claim.get("phase")
        if not isinstance(candidate, str):
            candidate = claim.get("name")
        phase = candidate if isinstance(candidate, str) else None
    if isinstance(phase, str) and phase:
        claimed[phase] = claimed.get(phase, 0) + 1

executed = {}
for entry in history:
    if not isinstance(entry, dict):
        continue
    phases = entry.get("phasesExecuted") or []
    if not isinstance(phases, list):
        continue
    for phase in phases:
        if isinstance(phase, str) and phase:
            executed[phase] = executed.get(phase, 0) + 1

if not claimed:
    print("NO_CLAIMS=1")
    sys.exit(0)

# An entirely ABSENT execution record is not evidence of fabrication — it is a
# packet that does not track executionHistory (planning-only and legacy shapes
# routinely omit it). A GAP inside a record that exists is the real signal. This
# check adjudicates the second and abstains on the first; without it, widening
# claim parsing turns every history-less planning packet into a false block.
if not executed:
    print("NO_HISTORY=1")
    sys.exit(0)

unbacked = sorted(p for p in claimed if executed.get(p, 0) == 0)
excess = sorted(
    f"{p}({claimed[p]} claim/{executed.get(p, 0)} run)"
    for p in claimed
    if executed.get(p, 0) > 0 and claimed[p] > executed.get(p, 0)
)

print(f"CLAIMED_PHASES={len(claimed)}")
if unbacked:
    print(f"UNBACKED={'|'.join(unbacked)}")
if excess:
    print(f"EXCESS={'|'.join(excess)}")
PY
)"

if echo "$claim_backing_analysis" | grep -q '^NO_CLAIMS=1'; then
  info "No completedPhaseClaims recorded — phase-claim backing check skipped"
elif echo "$claim_backing_analysis" | grep -q '^NO_HISTORY=1'; then
  info "No executionHistory recorded — phase-claim backing check abstains (an absent record is not evidence of an unbacked claim; a gap inside a present record still fails)"
elif [[ -z "$claim_backing_analysis" ]]; then
  info "completedPhaseClaims unreadable — phase-claim backing check skipped"
else
  cb_unbacked="$(echo "$claim_backing_analysis" | grep -E '^UNBACKED=' | head -n 1 | sed 's/^UNBACKED=//' || true)"
  cb_excess="$(echo "$claim_backing_analysis" | grep -E '^EXCESS=' | head -n 1 | sed 's/^EXCESS=//' || true)"
  cb_phases="$(echo "$claim_backing_analysis" | grep -E '^CLAIMED_PHASES=' | head -n 1 | sed 's/^CLAIMED_PHASES=//' || true)"

  if [[ -n "$cb_unbacked" ]]; then
    fail "completedPhaseClaims claims phase(s) with NO executionHistory entry behind them: $cb_unbacked — a phase cannot be claimed complete with no record that it ran"
  fi
  if [[ -n "$cb_excess" ]]; then
    warn "completedPhaseClaims holds more claims than recorded runs for: $cb_excess — each run should write its own executionHistory entry"
  fi
  if [[ -z "$cb_unbacked" ]] && [[ -z "$cb_excess" ]]; then
    pass "Every claimed phase has at least as many executionHistory runs as claims (${cb_phases:-0} phase(s))"
  fi
fi
echo ""

# =============================================================================
# CHECK 8: Test file existence — verify Test Plan files exist on disk
# =============================================================================
echo "--- Check 8: Test File Existence ---"

_check8_candidate_has_supported_suffix() {
  local candidate="$1"
  local stem=""
  local basename_stem=""

  case "$candidate" in
    ""|*[!A-Za-z0-9._/-]*) return 1 ;;
  esac

  case "$candidate" in
    *.spec.mjs) stem="${candidate%.spec.mjs}" ;;
    *.test.mjs) stem="${candidate%.test.mjs}" ;;
    *.spec) stem="${candidate%.spec}" ;;
    *.test) stem="${candidate%.test}" ;;
    *.rs) stem="${candidate%.rs}" ;;
    *.ts) stem="${candidate%.ts}" ;;
    *.tsx) stem="${candidate%.tsx}" ;;
    *.js) stem="${candidate%.js}" ;;
    *.jsx) stem="${candidate%.jsx}" ;;
    *.sh) stem="${candidate%.sh}" ;;
    *.bash) stem="${candidate%.bash}" ;;
    *.bats) stem="${candidate%.bats}" ;;
    *.py) stem="${candidate%.py}" ;;
    *.go) stem="${candidate%.go}" ;;
    *.java) stem="${candidate%.java}" ;;
    *.scala) stem="${candidate%.scala}" ;;
    *.dart) stem="${candidate%.dart}" ;;
    *) return 1 ;;
  esac

  basename_stem="${stem##*/}"
  [[ -n "$basename_stem" ]]
}

_check8_path_prefix_before_control() {
  local lexical_token="$1"
  local token_pattern='^([A-Za-z0-9._/-]+)(&&|\|\||;|$)'

  CHECK8_TOKEN_CANDIDATE=""
  if [[ "$lexical_token" =~ $token_pattern ]]; then
    CHECK8_TOKEN_CANDIDATE="${BASH_REMATCH[1]}"
    return 0
  fi
  return 1
}

_check8_candidate_from_block() {
  local block="$1"
  local bare_path_pattern='^[A-Za-z0-9._/-]+$'
  local candidate=""
  local first_word=""
  local token=""
  local token_index=0
  local options_open=1
  local -a block_words

  CHECK8_CANDIDATE=""
  while [[ "$block" == [[:blank:]]* ]]; do block="${block#?}"; done
  while [[ "$block" == *[[:blank:]] ]]; do block="${block%?}"; done
  [[ -n "$block" ]] || return 1

  IFS=$' \t' read -r -a block_words <<< "$block"
  first_word="${block_words[0]:-}"

  if [[ "$block" =~ $bare_path_pattern ]]; then
    candidate="$block"
  elif [[ "$first_word" == "bash" || "$first_word" == "sh" ]]; then
    token_index=1
    while [[ "$token_index" -lt "${#block_words[@]}" ]]; do
      token="${block_words[$token_index]}"
      if [[ "$options_open" -eq 1 ]]; then
        case "$token" in
          --)
            options_open=0
            token_index=$((token_index + 1))
            continue
            ;;
          -c|-[A-Za-z]*c[A-Za-z]*)
            return 1
            ;;
          -*)
            token_index=$((token_index + 1))
            continue
            ;;
        esac
      fi
      case "$token" in
        '&&'*|'||'*|';'*) return 1 ;;
      esac
      if _check8_path_prefix_before_control "$token"; then
        candidate="$CHECK8_TOKEN_CANDIDATE"
      fi
      break
    done
  elif _check8_path_prefix_before_control "$first_word"; then
    case "$CHECK8_TOKEN_CANDIDATE" in
      *.sh|*.bash|*.bats) candidate="$CHECK8_TOKEN_CANDIDATE" ;;
    esac
  fi

  if _check8_candidate_has_supported_suffix "$candidate"; then
    CHECK8_CANDIDATE="$candidate"
    return 0
  fi
  return 1
}

test_files_in_plan=()
for scope_path in ${scope_files[@]+"${scope_files[@]}"}; do
  [[ -f "$scope_path" ]] || continue
  while IFS= read -r line; do
    path=""
    backtick_marker='`'
    while IFS= read -r backtick_block; do
      block="${backtick_block#"$backtick_marker"}"
      block="${block%"$backtick_marker"}"
      if _check8_candidate_from_block "$block"; then
        path="$CHECK8_CANDIDATE"
        break
      fi
    done < <(printf '%s\n' "$line" | grep -oE '`[^`]*`' || true)
    if [[ -n "$path" ]] && [[ "$path" != "[path]" ]] && [[ ! "$path" =~ ^\[ ]]; then
      test_files_in_plan+=("$path")
    fi
  done < <(grep -E '^\|.*\|.*\|.*\|' "$scope_path" 2>/dev/null || true)
done

missing_test_files=0
if [[ ${#test_files_in_plan[@]} -gt 0 ]]; then
  for test_path in "${test_files_in_plan[@]}"; do
    if [[ -f "$test_path" ]]; then
      if [[ "$transition_audit_profile" == "planning-maturity-v1" ]]; then
        info "Planned test file already exists; physical existence is not used as planning-maturity proof: $test_path"
      elif ! check_is_applicable Check-8-file-existence; then
        info "Planned test file already exists; physical existence is not used as framework-proposal proof: $test_path"
      else
        pass "Test file exists: $test_path"
      fi
    elif [[ "$test_path" != */* ]]; then
      unique_match="$({ bubbles_pruned_find "$feature_dir/../.." -type f -name "$test_path" -print 2>/dev/null; } || true)"
      unique_match_count="$({ printf '%s\n' "$unique_match" | grep -c .; } || true)"
      if [[ "$unique_match_count" -eq 1 ]]; then
        warn "Test Plan uses basename-only path '$test_path'; uniquely resolved to $(echo "$unique_match" | sed "s#^$feature_dir/../..##")"
      elif [[ "$transition_audit_profile" == "planning-maturity-v1" ]]; then
        info "Future implementation-owned file is not physically required at planning maturity: $test_path"
        missing_test_files=$((missing_test_files + 1))
      else
        record_failed_check Check-8-contract
        fail "Test Plan references non-existent or non-resolvable file: $test_path"
        missing_test_files=$((missing_test_files + 1))
      fi
    elif [[ "$transition_audit_profile" == "planning-maturity-v1" ]]; then
      info "Future implementation-owned test file is not physically required at planning maturity: $test_path"
      missing_test_files=$((missing_test_files + 1))
    elif ! check_is_applicable Check-8-file-existence; then
      info "Future implementation-owned test file is not physically required for a framework proposal: $test_path"
      missing_test_files=$((missing_test_files + 1))
    else
      record_failed_check Check-8-file-existence
      fail "Test Plan references non-existent file: $test_path"
      missing_test_files=$((missing_test_files + 1))
    fi
  done
  if [[ "$missing_test_files" -gt 0 ]]; then
    if [[ "$transition_audit_profile" == "planning-maturity-v1" ]]; then
      info "NOT_APPLICABLE: Check-8-file-existence — $missing_test_files future implementation-owned test file(s) are absent"
    elif ! check_is_applicable Check-8-file-existence; then
      info "NOT_APPLICABLE: Check-8-file-existence — $missing_test_files future implementation-owned test file(s) are absent"
    else
      record_failed_check Check-8-file-existence
      fail "$missing_test_files of ${#test_files_in_plan[@]} test files from Test Plan DO NOT EXIST"
    fi
  elif [[ "$transition_audit_profile" == "planning-maturity-v1" ]]; then
    info "NOT_APPLICABLE: Check-8-file-existence — planning maturity validates test contracts, not delivery file presence"
  elif ! check_is_applicable Check-8-file-existence; then
    info "NOT_APPLICABLE: Check-8-file-existence — a framework proposal validates test contracts, not delivery file presence"
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
check9_failures_before="$failures"
# IMP-102 SCOPE-1 fix #3 (ADVISORY): count prose-only evidence blocks accepted
# this run so Check 9 can report the would-fail count without blocking (R1).
check9_advisory_count=0
checked_without_evidence=0
checked_with_evidence=0

# IMP-047 S-C (AC13): tool-log SEMANTIC admission.
#
# WHAT THIS REPLACED, AND WHY
# The previous rule admitted an exit-zero tool call when its command line shared
# TWO non-stopword tokens with a checked DoD item. Two tokens is a coincidence,
# not a proof: `npm run lint` and a DoD item about "lint the run-time config"
# overlap on `lint` and `run` while proving nothing about each other. Lexical
# coincidence presented as evidence is the exact fabrication shape the evidence
# rules exist to stop, so the token path is REMOVED rather than kept alongside.
#
# WHAT REPLACES IT
# Five bindings, all required, from bubbles/registry/scenario-states.yaml:
#   scenario  the DoD item POINTS AT a scenario (`Receipt: SCN-...`) and the
#             receipt is filed under that same scenario. Intent, not accident.
#   claim     the receipt's claim COVERS the DoD item: every content word of the
#             item appears in the claim. Directional and total, so a two-word
#             brush cannot satisfy it.
#   command   the receipt records the literal command that ran.
#   revision  the receipt cites the resolved source revision. Drift refuses,
#             because a receipt proves what the tree WAS.
#   outcome   exit zero, unless the item explicitly asserts an expected failure.
#
# Safe to call when no log exists or python3 is unavailable (returns 1). The
# markdown evidence paths (cases 1-3) are untouched and remain valid.
_tool_log_covers_dod_item() {
  local scope_dir="$1"
  local dod_line="$2"
  command -v python3 >/dev/null 2>&1 || return 1
  # Resolve repo root from the scope dir.
  local repo_root
  repo_root="$(cd "$scope_dir" && git rev-parse --show-toplevel 2>/dev/null || pwd)"
  local log_path="$repo_root/.specify/runtime/tool-calls.jsonl"
  [[ -f "$log_path" ]] || return 1
  local schema_path="$SCRIPT_DIR/../schemas/tool-call.schema.json"
  local source_revision
  source_revision="$(git -C "$repo_root" rev-parse --verify HEAD 2>/dev/null || true)"
  local spec_slug
  spec_slug="$(basename "$(cd "$scope_dir" && (cd .. 2>/dev/null && pwd) || pwd)")"
  # If the scope_dir IS the spec dir (single-file mode), use its basename.
  if [[ -f "$scope_dir/scopes.md" || -f "$scope_dir/spec.md" ]]; then
    spec_slug="$(basename "$scope_dir")"
  fi
  SCOPE_DIR="$scope_dir" SPEC_SLUG="$spec_slug" LOG_PATH="$log_path" DOD_LINE="$dod_line" \
  SCHEMA_PATH="$schema_path" SOURCE_REVISION="$source_revision" \
    python3 - <<'PY'
import json, os, re, sys
log_path = os.environ['LOG_PATH']
spec_slug = os.environ['SPEC_SLUG']
dod = os.environ['DOD_LINE']
source_revision = os.environ.get('SOURCE_REVISION', '').strip()

body = re.sub(r'^- \[[xX]\] ', '', dod)

# SCENARIO BINDING, caller side. Without an explicit pointer there is no claim
# of coverage to verify, and a guard that infers the pointer is back to guessing.
pointer = re.search(r'receipt:\s*([A-Za-z][A-Za-z0-9_.-]*)', body, re.IGNORECASE)
if not pointer:
    sys.exit(1)
wanted_scenario = pointer.group(1)

# The claim must cover the ITEM, so the pointer and any evidence anchor are
# removed first: they are addressing, not assertion.
claim_subject = re.sub(r'(?:\u2192\s*)?receipt:\s*[A-Za-z][A-Za-z0-9_.-]*', ' ', body, flags=re.IGNORECASE)
claim_subject = re.sub(r'(?:\u2192\s*)?evidence:\s*\S+', ' ', claim_subject, flags=re.IGNORECASE)

toks_re = re.compile(r'[a-zA-Z][a-zA-Z0-9._/-]{2,}')
STOP = {'the', 'and', 'for', 'with', 'this', 'that', 'from', 'into', 'have',
        'has', 'was', 'were', 'are', 'its', 'via', 'per', 'all', 'any', 'each'}

def content(text):
    return {t.lower() for t in toks_re.findall(text)} - STOP

item_toks = content(claim_subject)
if not item_toks:
    sys.exit(1)

# OUTCOME BINDING. An item that asserts an expected failure is proven by a
# non-zero exit; treating both as "exit 0" would make the outcome unfalsifiable.
expects_failure = bool(re.search(r'\b(fails?|failing|refus\w+|rejects?|blocked)\b',
                                 claim_subject, re.IGNORECASE))

_validator = None
_schema_path = os.environ.get('SCHEMA_PATH', '')
if _schema_path and os.path.isfile(_schema_path):
    try:
        from jsonschema import Draft7Validator
        with open(_schema_path) as _sf:
            _validator = Draft7Validator(json.load(_sf))
    except Exception:
        _validator = None

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
            # A schema-invalid (e.g. forged) line is NON-matching when a
            # validator is available.
            if _validator is not None and next(_validator.iter_errors(d), None) is not None:
                continue
            # An entry with an EMPTY spec names nothing and must not bleed into
            # every spec's evidence.
            sf = (d.get('spec') or '').strip()
            if not sf:
                continue
            if sf != spec_slug and not sf.startswith(spec_slug.split('-', 1)[0]):
                continue

            binding = d.get('scenarioBinding')
            if not isinstance(binding, dict):
                continue
            if (binding.get('scenarioId') or '').strip() != wanted_scenario:
                continue

            # COMMAND BINDING.
            if not (d.get('cmd') or '').strip():
                continue

            # SOURCE-REVISION BINDING. Drift invalidates the receipt.
            rev = (binding.get('sourceRevision') or '').strip()
            if not rev or (source_revision and rev != source_revision):
                continue

            # OUTCOME BINDING.
            exit_code = d.get('exitCode')
            if not isinstance(exit_code, int):
                continue
            if expects_failure:
                if exit_code == 0:
                    continue
            elif exit_code != 0:
                continue

            # CLAIM BINDING. Total coverage, not partial overlap.
            claim = (binding.get('claim') or '').strip()
            if not claim:
                continue
            if item_toks - content(claim):
                continue

            sys.exit(0)
except FileNotFoundError:
    sys.exit(1)
sys.exit(1)
PY
}

# IMP-027 SCOPE-3 (EV-1): does a DoD item assert an EXECUTION OUTCOME?
#
# README's evidence guarantee is specifically about execution claims — 'a
# narrative "all tests pass" with no terminal output is rejected as
# fabrication'. It is NOT a claim that every DoD item must be receipted.
# Documentation, design-decision, and attestation items legitimately carry
# prose, and failing those would manufacture false failures at scale.
#
# So the command-output requirement is CLAIM-TYPED, not global. This matcher
# decides which side of that line an item falls on. It is deliberately anchored
# to the VERB+SUBJECT shape of an execution assertion rather than to bare
# keywords: "documented the test strategy" must NOT match, while "unit tests
# pass" must.
dod_item_claims_execution() {
  local item_text="${1:-}"
  [[ -z "$item_text" ]] && return 1
  local probe
  probe="$(printf '%s' "$item_text" | tr '[:upper:]' '[:lower:]')"

  # Strip the Evidence: pointer — anchor slugs routinely contain words like
  # "test" and would otherwise decide the claim type by accident.
  probe="${probe%%→ evidence:*}"
  probe="${probe%%evidence:*}"

  # Negative guard first: an item ABOUT execution artifacts that does not itself
  # assert an execution outcome (authoring, documenting, planning, designing).
  if [[ "$probe" =~ (documented|document|describe[sd]?|plan(ned|s)?\ |design(ed|s)?\ |written|writes|author(ed|s)?|specif(y|ied|ies)|record(ed|s)?\ the) ]]; then
    # ...unless it ALSO asserts an outcome ("tests written and passing").
    if [[ ! "$probe" =~ (pass(es|ing|ed)?|green|succeed(s|ed|ing)?|exit\ code\ 0|0\ failures|clean) ]]; then
      return 1
    fi
  fi

  # Positive: an execution SUBJECT paired with an outcome/imperative.
  if [[ "$probe" =~ (test|suite|build|compil|lint|clippy|fmt|format|coverage|benchmark|selftest|e2e|integration|smoke|stress|migration|deploy|guard|scan|audit) ]] &&
     [[ "$probe" =~ (pass(es|ing|ed)?|run(s|ning)?|execut(e|ed|es|ing)|succeed(s|ed|ing)?|green|clean|exit\ code|0\ (failures|errors|warnings)|no\ (failures|errors|warnings)|complete[sd]?|verif(y|ied|ies)) ]]; then
    return 0
  fi
  return 1
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
  local dod_item_text="${3:-}"  # IMP-027 SCOPE-3: decides the claim type
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
  # Find the anchor — match either an HTML anchor <a name="X"> / <a id="X">,
  # an explicit {#anchor} attribute, or a Markdown heading whose GitHub slug
  # matches.
  #
  # IMP-102 fix (Defect 3): `<a id="X">` is the modern HTML anchor form and the
  # shape agents naturally emit, but the matcher previously accepted ONLY
  # `<a name="X">`, so a perfectly valid anchor resolved as "missing" and the
  # DoD item hard-failed. Matching the tag first and the attribute second also
  # tolerates attribute order (`<a class="x" id="y">`). This strictly REDUCES
  # false failures — it cannot newly fail anything that resolves today.
  local anchor_line
  anchor_line="$(awk -v a="$anchor_lower" '
    BEGIN { IGNORECASE=1 }
    /<a[[:space:]]/ {
      hay = tolower($0)
      if (hay ~ "name=\""a"\"" || hay ~ "id=\""a"\"") { print NR; exit }
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
  # Count non-blank lines from anchor_line+1 until next heading or EOF.
  #
  # IMP-102 fix (Defect 2): the end-of-block scan is FENCE-AWARE. A pasted shell
  # comment inside the evidence fence (e.g. `# TP-03-01 rollback accounting`)
  # matches /^#+[[:space:]]/ and previously terminated the block early, so the
  # ≥10-non-blank-line rule measured a fraction of the real evidence and emitted
  # a FALSE block-too-short failure. Only a `#` heading OUTSIDE a fenced block
  # ends the evidence window. Fence state is tracked from line 1 so it is
  # correct by the time the anchor line is reached. This strictly REDUCES false
  # failures — the window can only grow, never shrink.
  local end_line
  end_line="$(awk -v start="$anchor_line" '
    {
      probe = $0
      sub(/^[[:space:]]+/, "", probe)
      if (probe ~ /^```/) { in_fence = !in_fence; next }
    }
    NR>start && !in_fence && /^#+[[:space:]]/ { print NR; exit }
  ' "$report_path")"
  [[ -z "$end_line" ]] && end_line="$(wc -l < "$report_path")"
  local block_text block_lines
  block_text="$(sed -n "$((anchor_line+1)),${end_line}p" "$report_path")"
  block_lines="$(printf '%s\n' "$block_text" | grep -cE '\S' || true)"
  if [[ "${block_lines:-0}" -ge 10 ]]; then
    # A resolved ≥10-line block carrying NO command-output signature.
    #
    # IMP-102 SCOPE-1 fix #3 made this an ADVISORY because documentation and
    # attestation DoD items legitimately use prose.
    #
    # IMP-027 SCOPE-3 (EV-1) narrows that blanket permission WITHOUT
    # reintroducing false failures: the permission stands for items that do not
    # assert an execution outcome, and is WITHDRAWN for items that do. This is
    # what closes the gap between README's stated guarantee and the code — a
    # narrative "all tests pass" with no terminal output is now refused, while
    # "architecture decision recorded in design.md" still passes on prose.
    #
    # BUG-009: this MUST NOT pipe into `grep -q`. `grep -q` exits on the first
    # match, so a block larger than the 64 KiB pipe buffer kills the still-
    # writing `printf` with SIGPIPE; under `set -o pipefail` the pipeline then
    # reports 141 even though the pattern WAS found, and the leading `!` turned
    # that into a false "prose-only" block. A herestring is a file, not a pipe.
    if ! grep -qE '```|Exit Code:|^[[:space:]]*\$ |Executed:|Command:' <<< "$block_text"; then
      if dod_item_claims_execution "$dod_item_text"; then
        check9_prose_execution_anchor="$anchor"
        return 1
      fi
      check9_advisory_count=$((${check9_advisory_count:-0} + 1))
      info "Check-9 ADVISORY: evidence block for anchor '#${anchor}' in $(basename "$report_path") has no command-output signature (prose-only); accepted as documentation/attestation evidence"
    fi
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

for scope_path in ${scope_files[@]+"${scope_files[@]}"}; do
  [[ -f "$scope_path" ]] || continue
  scope_dir="$(dirname "$scope_path")"
  # IMP-102 SCOPE-1 fix #7: resolve each identical checked line to ITS OWN
  # occurrence — the Nth identical `- [x]` line resolves to the Nth matching
  # line number instead of always the first — so a duplicated DoD line cannot
  # borrow the first occurrence's evidence window.
  declare -A _c9_line_seen=()
  while IFS= read -r line; do
    _c9_occ=$(( ${_c9_line_seen["$line"]:-0} + 1 ))
    _c9_line_seen["$line"]=$_c9_occ
    item_line_num="$({ grep -nF -- "$line" "$scope_path" | sed -n "${_c9_occ}p" | cut -d: -f1; } || true)"
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
          check9_prose_execution_anchor=""
          if resolve_evidence_by_reference "$scope_dir" "$link_target" "$line"; then
            checked_with_evidence=$((checked_with_evidence + 1))
          else
            checked_without_evidence=$((checked_without_evidence + 1))
            if [[ -n "${check9_prose_execution_anchor:-}" ]]; then
              fail "DoD item [x] asserts an EXECUTION outcome but its evidence block '#${check9_prose_execution_anchor}' contains no command output (prose-only) in $(relative_artifact_path "$scope_path"): $(echo "$line" | head -c 80)"
            else
              fail "DoD item [x] references '$link_target' but anchor missing OR block <10 non-blank lines in $(relative_artifact_path "$scope_path"): $(echo "$line" | head -c 80)"
            fi
          fi
        else
          # IMP-102 SCOPE-1 fix #1: a marker WITHOUT a resolvable
          # report.md#anchor markdown link passes ONLY if it still points at a
          # report.md reference (bare `report.md[#anchor]`) OR carries an inline
          # evidence block OR a plain report.md markdown link. A truly-bare
          # marker (e.g. `→ Evidence: done`) with none of these is FABRICATION.
          if [[ "$line" == *"report.md"* ]] || [[ "$_c9_inline" -eq 1 ]] || [[ "$_c9_link" -eq 1 ]]; then
            checked_with_evidence=$((checked_with_evidence + 1))
          else
            checked_without_evidence=$((checked_without_evidence + 1))
            fail "DoD item [x] has a bare Evidence marker with no report.md reference or inline evidence block in $(relative_artifact_path "$scope_path"): $(echo "$line" | head -c 80)"
          fi
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
          check9_prose_execution_anchor=""
          if resolve_evidence_by_reference "$scope_dir" "$link_target" "$line"; then
            checked_with_evidence=$((checked_with_evidence + 1))
          else
            checked_without_evidence=$((checked_without_evidence + 1))
            if [[ -n "${check9_prose_execution_anchor:-}" ]]; then
              fail "DoD item [x] asserts an EXECUTION outcome but its evidence block '#${check9_prose_execution_anchor}' contains no command output (prose-only) in $(relative_artifact_path "$scope_path"): $(echo "$line" | head -c 80)"
            else
              fail "DoD item [x] links '$link_target' but anchor missing OR block <10 non-blank lines in $(relative_artifact_path "$scope_path"): $(echo "$line" | head -c 80)"
            fi
          fi
        else
          # Plain report.md link with no anchor — IMP-102 SCOPE-1 fix #2:
          # require the linked report.md to EXIST and carry ≥10 non-blank lines.
          # An empty/near-empty report.md is not evidence merely because the
          # file is present.
          rel_report="${link_target##*/}"
          [[ -z "$rel_report" ]] && rel_report="report.md"
          plain_report_path="$scope_dir/$rel_report"
          [[ -f "$plain_report_path" ]] || plain_report_path="$scope_dir/report.md"
          plain_report_lines=0
          [[ -f "$plain_report_path" ]] && plain_report_lines="$(grep -cE '\S' "$plain_report_path" 2>/dev/null || echo 0)"
          if [[ -f "$plain_report_path" ]] && [[ "${plain_report_lines:-0}" -ge 10 ]]; then
            checked_with_evidence=$((checked_with_evidence + 1))
          else
            checked_without_evidence=$((checked_without_evidence + 1))
            fail "DoD item [x] links report.md but it is missing or has <10 non-blank lines in $scope_dir: $(echo "$line" | head -c 80)"
          fi
        fi
      # 3. Inline evidence block within next 15 lines (v4.0.x behavior)
      elif [[ "$_c9_inline" -eq 1 ]]; then
        # IMP-102 SCOPE-1 fix #3 (ADVISORY): a ≥10-line inline block with no
        # fenced command-output signature is accepted as prose documentation /
        # attestation evidence, but counted as an advisory (R1, non-blocking).
        _c9_inline_nonblank="$(printf '%s\n' "$next_lines" | grep -cE '\S' || true)"
        # BUG-009: herestring, not a pipe — see the same fix in
        # resolved_block_has_execution_signature. `grep -q` closing the pipe
        # early SIGPIPEs `printf`, and `pipefail` turns a FOUND pattern into a
        # non-zero status that `!` reads as "no command-output signature".
        if [[ "${_c9_inline_nonblank:-0}" -ge 10 ]] && ! grep -qE '```|Exit Code:|^[[:space:]]*\$ |Executed:|Command:' <<< "$next_lines"; then
          check9_advisory_count=$((${check9_advisory_count:-0} + 1))
          info "Check-9 ADVISORY: inline evidence block in $(relative_artifact_path "$scope_path") has no command-output signature (prose-only); accepted as documentation/attestation evidence"
        fi
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
  done < <(grep -E '^\- \[[xX]\] ' "$scope_path" 2>/dev/null || true)
done

if [[ "$checked_without_evidence" -eq 0 ]] && [[ "$checked_with_evidence" -gt 0 ]]; then
  pass "All $checked_with_evidence checked DoD items across resolved scope files have evidence blocks"
elif [[ "$checked_with_evidence" -eq 0 ]] && [[ "$total_checked" -gt 0 ]]; then
  fail "ALL checked DoD items across resolved scope files lack evidence blocks — BULK FABRICATION DETECTED"
fi
if [[ "${check9_advisory_count:-0}" -gt 0 ]]; then
  info "Check-9 advisory: $check9_advisory_count prose-only evidence block(s) accepted this run (would-fail count under a future blocking command-output policy; IMP-102 SCOPE-1 R1 advisory)"
fi
if [[ "$failures" -gt "$check9_failures_before" ]]; then
  record_failed_check Check-9-evidence
fi
echo ""

# =============================================================================
# CHECK 10: Template placeholder detection
# =============================================================================
echo "--- Check 10: Template Placeholder Detection ---"
for scope_path in ${scope_files[@]+"${scope_files[@]}"}; do
  [[ -f "$scope_path" ]] || continue
  template_hits="$({ grep -cnE '\[ACTUAL terminal output|\[exact cmd\]|\[actual exit code\]|\[ACTUAL output|\[command \+ output|\[cmd\]|\[PASTE VERBATIM terminal output|\[PASTE VERBATIM.*output here' "$scope_path"; } || true)"
  if [[ "$template_hits" -gt 0 ]]; then
    fail "$(relative_artifact_path "$scope_path") contains $template_hits unfilled template placeholders — FABRICATION"
  else
    pass "No template placeholders in $(relative_artifact_path "$scope_path")"
  fi
done

for report_path in ${report_files[@]+"${report_files[@]}"}; do
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
  record_failed_check Check-11-structure
  fail "No report.md files were resolved for this feature"
fi

implementation_phase_claim_count="$(jq -r '
  [
    ((.execution.completedPhaseClaims // [])[]? | if type == "string" then . else (.phase // empty) end),
    ((.certification.certifiedCompletedPhases // [])[]? | if type == "string" then . else (.phase // empty) end),
    ((.executionHistory // [])[]? | .phase // empty),
    (.execution.currentPhase // empty),
    (.currentPhase // empty)
  ]
  | map(select(. == "implement" or . == "test"))
  | length
' "$state_file" 2>/dev/null || printf '0')"

# BUG-005: precompiled ERE patterns for the 8 evidence-signal categories used by
# the per-line legitimacy scan below. Single-quoted so every regex metacharacter
# (incl. `$`, `[`, `(`, backslashes) is literal to bash `[[ =~ ]]`. Categories
# i/ii/iv/v/vii are case-INSENSITIVE (original grep -qiE); iii/vi/viii are
# case-SENSITIVE (original grep -qE) — see the per-line tests below.
_c11_sig_i_re='(passed|failed|ok$| PASS | FAIL |test result:|Tests:.*suites|✓|✗|PASSED|FAILED)'
_c11_sig_ii_re='(exit code|Exit Code:|error\[|warning\[|Compiling |Finished |error:|warning:|WARN |ERROR |INFO )'
_c11_sig_iii_re='(([a-zA-Z0-9_-]+/)?[a-zA-Z0-9_.-]+\.(rs|py|ts|tsx|js|jsx|mjs|cjs|go|sh|sql|toml|yaml|yml|json|proto|md|dart|kt|kts|swift|java|scala|rb|cs|cpp|cc|h|hpp|xml|gradle|tf|ex|exs)|\./)'
_c11_sig_iv_re='(in [0-9]+(\.[0-9]+)?(s|ms|m)|elapsed|finished in|Duration|[0-9]+\.[0-9]+s$)'
_c11_sig_v_re='(cargo |npm |pytest|go test|jest |playwright|vitest|running [0-9]+ test|test result:)'
_c11_sig_vi_re='[0-9]+ (passed|failed|errors?|warnings?|skipped|ignored|tests?)'
_c11_sig_vii_re='(HTTP/|status.*[0-9]{3}|curl |GET /|POST /|PUT /|DELETE /|Content-Type)'
_c11_sig_viii_re='(^[dl-][rwx-]{9} |^[0-9]+:|^\$ |^> )'

for report_path in ${report_files[@]+"${report_files[@]}"}; do
  if [[ ! -f "$report_path" ]]; then
    fail "Missing report file: $(relative_artifact_path "$report_path")"
    continue
  fi

  # IMP-047 S-B: the always-required section list is READ from
  # bubbles/registry/report-sections.yaml, the same authority artifact-lint.sh
  # reads and the report template is generated from. It used to be restated
  # here, which is how the guard, the lint, the autofix script and the template
  # arrived at four different answers.
  required_headers=()
  _rs_resolver="$SCRIPT_DIR/report-sections-resolve.sh"
  if [[ ! -f "$_rs_resolver" ]]; then
    fail "report-sections-resolve.sh is missing; the report section contract cannot be read"
  elif ! _rs_facts="$(bash "$_rs_resolver" 2>&1)"; then
    fail "cannot read bubbles/registry/report-sections.yaml: $_rs_facts"
  else
    while IFS= read -r _rs_line; do
      _rs_heading="${_rs_line#always=}"
      _rs_shallow="${_rs_heading##*|}"
      _rs_heading="${_rs_heading%|*}"
      if [[ "$_rs_shallow" == "yes" ]]; then
        required_headers+=("^###[[:space:]]+${_rs_heading}|^##[[:space:]]+${_rs_heading}")
      else
        required_headers+=("^###[[:space:]]+${_rs_heading}")
      fi
    done < <(printf '%s\n' "$_rs_facts" | grep '^always=' || true)
    # An empty requirement set would silently pass every report. That is the
    # PD-04 false-PASS shape, so it is a failure, not a skip.
    [[ ${#required_headers[@]} -gt 0 ]] ||
      fail "bubbles/registry/report-sections.yaml declares no alwaysRequired sections"
  fi
  for header in ${required_headers[@]+"${required_headers[@]}"}; do
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
    if [[ "$transition_audit_profile" == "planning-maturity-v1" \
      && "$total_checked" -eq 0 \
      && "$done_scopes" -eq 0 \
      && "$state_completed_scopes_count" -eq 0 \
      && "$implementation_phase_claim_count" -eq 0 ]]; then
      info "Honest planning report has zero execution-evidence blocks: $(relative_artifact_path "$report_path")"
    elif [[ "$transition_audit_profile" == "planning-maturity-v1" ]]; then
      record_failed_check Check-11-execution-honesty
      fail "$(relative_artifact_path "$report_path") has ZERO evidence code blocks but state or scope artifacts claim completed delivery work"
    elif ! check_is_applicable Check-11-execution-evidence; then
      info "Framework proposal report has zero execution-evidence blocks: $(relative_artifact_path "$report_path")"
    else
      record_failed_check Check-11-execution-evidence
      fail "$(relative_artifact_path "$report_path") has ZERO evidence code blocks — no execution evidence exists"
    fi
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
if [[ "$transition_audit_profile" == "planning-maturity-v1" ]]; then
  info "NOT_APPLICABLE: Check-11-execution-evidence — honest unimplemented scope reports need no delivery execution block"
elif ! check_is_applicable Check-11-execution-evidence; then
  info "NOT_APPLICABLE: Check-11-execution-evidence — a framework proposal runs no delivery execution, so it produces no delivery execution block"
fi
echo ""

# =============================================================================
# CHECK 12: Duplicate evidence detection (Gate G021)
# =============================================================================
# IMP-102 fix (Defect 1): Check 12 previously had TWO independent blindnesses,
# either sufficient alone to keep it from ever firing on a real artifact:
#   1. It iterated ONLY `scope_files` and never opened `report_files` — but the
#      evidence-by-reference convention (see resolve_evidence_by_reference)
#      puts the fenced blocks in report.md.
#   2. It matched ONLY 4-space-indented fences, while real artifacts write
#      fences at column 0 almost exclusively.
# The fix scans BOTH surfaces and recognises fences at ANY indentation, but it
# does so at TWO severities so a previously-blind blocking gate cannot
# retro-break already-certified downstream packets:
#
#   * LEGACY surface — scope files, 4-space-indented fences — stays BLOCKING
#     with byte-identical detection semantics. Zero behaviour change.
#   * NEWLY COVERED surface — scope + report files, fences at any indentation —
#     is ADVISORY (info + counter, never `fail`). Some repetition is
#     legitimate (e.g. one shared environment-context block quoted by sibling
#     scopes), and downstream repos carry `done` packets certified while this
#     surface was blind. This mirrors the framework's own established
#     precedent for newly-activated enforcement (see `check9_advisory_count`
#     above: "Advisory-for-one-release, NOT blocking"). Promote the advisory
#     surface to blocking in a later release once downstream repos have
#     drained the backlog.
_c12_fence_any_re='^[[:space:]]*```'

# Detect an exact-duplicate fenced evidence block within a single artifact.
#   $1 = file path
#   $2 = fence mode:
#        "legacy-4space" — open on a 4-space `    ```` prefix, close on an
#                          exact `    ```` line (the historical semantics)
#        "any-indent"    — toggle on any fence line at any indentation,
#                          covering both ```lang openers and bare ``` closers
# Returns 0 when a duplicate block is found, 1 otherwise.
#
# Block text is compared directly instead of hashed: it yields the identical
# equality relation the previous md5 implementation did, removes a GNU-only
# `md5sum` dependency (macOS ships `md5`, not `md5sum`), and drops two forks
# per block. The concatenation shape is byte-for-byte what was hashed before.
c12_has_duplicate_evidence_block() {
  local file_path="$1"
  local fence_mode="$2"
  local blocks=()
  local in_evidence=0
  local current_evidence=""
  local line=""
  local prev_block=""
  local is_open=0
  local is_close=0

  while IFS= read -r line; do
    is_open=0
    is_close=0
    if [[ "$fence_mode" == "legacy-4space" ]]; then
      # BUG-005: bash glob builtins replace per-line echo|grep fence forks.
      [[ "$line" == '    ```'* ]] && is_open=1
      [[ "$line" == '    ```' ]] && is_close=1
    elif [[ "$line" =~ $_c12_fence_any_re ]]; then
      is_open=1
      is_close=1
    fi

    if [[ "$in_evidence" -eq 0 ]] && [[ "$is_open" -eq 1 ]]; then
      in_evidence=1
      current_evidence=""
    elif [[ "$in_evidence" -eq 1 ]] && [[ "$is_close" -eq 1 ]]; then
      in_evidence=0
      if [[ -n "$current_evidence" ]]; then
        for prev_block in ${blocks[@]+"${blocks[@]}"}; do
          if [[ "$current_evidence" == "$prev_block" ]]; then
            return 0
          fi
        done
        blocks+=("$current_evidence")
      fi
    elif [[ "$in_evidence" -eq 1 ]]; then
      current_evidence="${current_evidence}${line}"
    fi
  done < "$file_path"

  return 1
}

echo "--- Check 12: Duplicate Evidence Detection ---"
c12_advisory_count=0

for scope_path in ${scope_files[@]+"${scope_files[@]}"}; do
  [[ -f "$scope_path" ]] || continue
  if c12_has_duplicate_evidence_block "$scope_path" "legacy-4space"; then
    fail "Duplicate evidence blocks detected in $(relative_artifact_path "$scope_path") — COPY-PASTE FABRICATION"
    continue
  fi

  pass "No duplicate evidence blocks in $(relative_artifact_path "$scope_path")"
  if c12_has_duplicate_evidence_block "$scope_path" "any-indent"; then
    c12_advisory_count=$((c12_advisory_count + 1))
    info "Check-12 ADVISORY: duplicate evidence block in $(relative_artifact_path "$scope_path") on the any-indentation fence surface — copy-paste fabrication indicator, NOT blocking this release (Gate G021 newly-covered surface)"
  fi
done

for report_path in ${report_files[@]+"${report_files[@]}"}; do
  [[ -f "$report_path" ]] || continue
  if c12_has_duplicate_evidence_block "$report_path" "any-indent"; then
    c12_advisory_count=$((c12_advisory_count + 1))
    info "Check-12 ADVISORY: duplicate evidence block in $(relative_artifact_path "$report_path") on the any-indentation fence surface — copy-paste fabrication indicator, NOT blocking this release (Gate G021 newly-covered surface)"
  fi
done

if [[ "$c12_advisory_count" -gt 0 ]]; then
  info "Check-12 advisory: $c12_advisory_count artifact(s) carry duplicate evidence blocks on the newly-covered any-indentation surface (would-fail count under a future blocking policy)"
fi
echo ""

# =============================================================================
# CHECK 13: Run artifact lint as final cross-check
# =============================================================================
echo "--- Check 13: Artifact Lint ---"
lint_script="$SCRIPT_DIR/artifact-lint.sh"
# Check 13 is fail-closed, so "did not complete" and "completed and rejected"
# must be reported as different things. Conflating them indicts a timing
# artifact as a content defect: a large spec whose lint legitimately exits 0 can
# exceed the cap under load and be reported as a lint FAILURE it does not have,
# which sends the reader hunting for findings that do not exist. The cap was 60s
# and is load-sensitive in practice (the same spec measured 32s idle and 73-90s
# under concurrent load), so the default is raised and made overridable.
artifact_lint_timeout="${BUBBLES_ARTIFACT_LINT_TIMEOUT:-300}"
if [[ -f "$lint_script" ]]; then
  lint_rc=0
  BUBBLES_WORKFLOWS_FILE="$workflow_registry_file" bubbles_run_with_timeout "$artifact_lint_timeout" bash "$lint_script" "$feature_dir" > /dev/null 2>&1 || lint_rc=$?
  if [[ "$lint_rc" -eq 0 ]]; then
    pass "Artifact lint passes (exit 0)"
  elif [[ "$lint_rc" -eq 124 ]]; then
    if [[ "$is_test_fixture_dir" == "true" ]]; then
      warn "Artifact lint TIMED OUT after ${artifact_lint_timeout}s for tests/fixtures target; not blocking fixture acceptance"
    else
      fail "Artifact lint did NOT COMPLETE within ${artifact_lint_timeout}s (exit 124) — this is a TIMEOUT, not a lint failure; re-run 'bash bubbles/scripts/artifact-lint.sh $feature_dir' directly, or raise BUBBLES_ARTIFACT_LINT_TIMEOUT"
    fi
  elif [[ "$is_test_fixture_dir" == "true" ]]; then
    warn "Artifact lint subprocess failed for tests/fixtures target after direct guard artifact checks passed; not blocking fixture acceptance"
  else
    fail "Artifact lint FAILED (exit $lint_rc) — run 'bash bubbles/scripts/artifact-lint.sh $feature_dir' for details"
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

  for rpt_path in ${report_files[@]+"${report_files[@]}"}; do
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
for scope_path in ${scope_files[@]+"${scope_files[@]}"}; do
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
      if grep -qE '"implement"' <<< "$state_completed_phases_block"; then
        has_implement="true"
      fi
      if grep -qE '"test"' <<< "$state_completed_phases_block"; then
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

if [[ "$transition_audit_profile" == "planning-maturity-v1" ]]; then
  # A planning-maturity transition (e.g. -> specs_hardened) certifies a PLAN,
  # not a delivered implementation. A planning-only spec (product-to-planning /
  # planMaturityOnly) describes future work by nature and legitimately uses
  # forward-looking domain terminology — e.g. a real MVP-surface / feature name
  # such as "Authorized Outcome Follow-Up". The context-free deferral scan would
  # flag such legitimate domain labels as "deferred work", so it is category-
  # inappropriate here and is deferred to the delivery-completion (done)
  # transition — matching how Check 4 (DoD completion) and Check 3E (scenario-
  # first TDD, Gate G060) treat this profile. Delivery enforcement is unchanged.
  info "NOT_APPLICABLE: Check-18 deferral-language scan — planning maturity describes a plan of future work, so forward-looking domain terminology is category-appropriate; deferral-language enforcement is deferred to the delivery-completion transition (Gate G040)"
elif [[ "$state_status" == "done_with_concerns" && "$(json_first_bool "legacyStatusCompatibility" "$state_file" || true)" == "true" ]]; then
  info "Check 18 skipped: state.json status is legacy read-only 'done_with_concerns' with legacyStatusCompatibility:true (Gate G040/G092)"
else
  # NOTE on the `placeholder` term (Gate G040 false-positive class). Every other
  # term in this list is prose that ADMITS deferral ("deferred", "out of scope",
  # "skip for now"). The bare noun `placeholder` is not: it is ordinary UI, DOM
  # and test vocabulary, and it appears most often in artifacts that FORBID one
  # — "the empty state renders with no placeholder card", "do not synthesise a
  # placeholder item". Matching the bare noun therefore flagged prose asserting
  # the exact opposite of deferral. It is narrowed to admission-bearing forms
  # ("is a placeholder", "placeholder value/until/for now"), which still catch a
  # genuine "this is a placeholder until X" admission while ignoring a noun that
  # merely names an artifact. Guarded by two selftest cases below: a negative
  # (prohibition prose must NOT block) and its adversarial twin (a real
  # admission MUST still block), so the narrowing cannot silently disable it.
  deferral_pattern='deferred|defer to|deferred to|future scope|future work|future iteration|follow-up|follow up|followup|out of scope|not in scope|beyond scope|will address later|address later|revisit later|separate ticket|separate issue|separate PR|tracked separately|handled separately|punt\b|punted|postpone|postponed|skip for now|skipped for now|not implemented yet|not yet implemented|(is|are|was|were|remains?|stays?|left|leaving)[[:space:]]+(still[[:space:]]+)?an?[[:space:]]+placeholder|placeholder[[:space:]]+(value|until|for now)|temporary workaround'
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

  for scope_path in ${scope_files[@]+"${scope_files[@]}"}; do
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

  # Also scan report files for deferral language.
  #
  # Certifying-window boundary (report.md ONLY, opt-in, at most ONE per file),
  # mirroring artifact-lint.sh Check 3: a single out-of-fence
  # <!-- bubbles:certifying-window-begin --> marker splits report.md into a
  # FROZEN prior-window history region (every line BEFORE the marker) and the
  # current certifying window (every line AFTER it). Pre-marker lines are
  # suppressed from this G040 scan — they were authored and validated in prior
  # specialist rounds and the append-only audit rule forbids rewriting them, so
  # a current-window transition MUST NOT re-adjudicate frozen prior history.
  # INTEGRITY (mirrors artifact-lint exactly): the exemption is opt-in PER FILE
  # (a marker-less report.md is enforced in FULL — the marker can never silently
  # disable G040 fleet-wide); >1 marker fails loud (ambiguous window start) AND
  # grants NO exemption (falls through to full enforcement); only report.md
  # targets are affected (the scope.md scan above is unchanged); post-marker
  # (current-window) enforcement is UNCHANGED and still strict.
  deferral_strip_report_awk='
    BEGIN { before_window = bw }
    /^```/ || /^    ```/ { in_block = !in_block; next }
    /<!-- bubbles:g040-skip-begin -->/ { skip = 1; next }
    /<!-- bubbles:g040-skip-end -->/ { skip = 0; next }
    !in_block && /<!-- bubbles:certifying-window-begin -->/ { before_window = 0; next }
    !in_block && !skip && !before_window { print }
  '
  for rpt_path in ${report_files[@]+"${report_files[@]}"}; do
    [[ -f "$rpt_path" ]] || continue

    # Resolve the certifying-window posture for THIS report.md (report-only).
    rpt_before_window=0
    cw_marker_count="$(grep -cF -- '<!-- bubbles:certifying-window-begin -->' "$rpt_path" || true)"
    if [[ "$cw_marker_count" -gt 1 ]]; then
      # Ambiguous window start: fail loud AND grant no exemption — bw stays 0 so
      # the whole file (pre-marker prose included) is fully enforced.
      fail "Multiple <!-- bubbles:certifying-window-begin --> markers ($cw_marker_count) in ${rpt_path#$feature_dir/} — at most one is allowed (it marks the single current certifying-window start) (Gate G040)"
    elif [[ "$cw_marker_count" -eq 1 ]]; then
      rpt_before_window=1
      cw_marker_line="$(grep -nF -- '<!-- bubbles:certifying-window-begin -->' "$rpt_path" | head -1 | cut -d: -f1)"
      info "Skipped $((cw_marker_line - 1)) lines before <!-- bubbles:certifying-window-begin --> (prior-window history) in ${rpt_path#$feature_dir/} (Gate G040)"
    fi

    report_deferral_hits="$({
      awk -v bw="$rpt_before_window" "$deferral_strip_report_awk" "$rpt_path" | grep -iE "$deferral_pattern" | grep -viE "$deferral_exclusion_pattern" | wc -l || true
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

for rpt_path in ${report_files[@]+"${report_files[@]}"}; do
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
for scope_path in ${scope_files[@]+"${scope_files[@]}"}; do
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
for scope_path in ${scope_files[@]+"${scope_files[@]}"}; do
  [[ -f "$scope_path" ]] || continue

  # Collect all evidence blocks as separate entries
  evidence_blocks=()
  in_evidence=0
  current_block=""
  while IFS= read -r line; do
    if [[ "$in_evidence" -eq 0 ]] && grep -qE '^    ```' <<< "$line"; then
      in_evidence=1
      current_block=""
    elif [[ "$in_evidence" -eq 1 ]] && grep -qE '^    ```$' <<< "$line"; then
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
      # NOTE: `-e` is REQUIRED. Without it, any evidence line beginning with '-'
      # (a markdown bullet, an SQL '--' comment, a diff '-' line) is parsed by
      # grep as an OPTION, which exits 2. Exit 2 was then read as "not shared",
      # undercounting shared_lines and making this fabrication gate FAIL OPEN.
      shared_lines=0
      while IFS= read -r a_line; do
        [[ -z "$a_line" ]] && continue
        if grep -qF -e "$a_line" <<< "$block_b"; then
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
  if grep -qE "^($spec_review_required_modes)$" <<< "$state_workflow_mode"; then
    if grep -qE '"spec-review"' <<< "$state_completed_phases_block"; then
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
# The matcher itself lives in scenario-match-lib.sh (BUG-004), which is the ONE
# implementation of the G068 rule. This check consumes it with the
# `structural-strict` id policy: SCN ids only, and a scenario that carries an
# SCN id whose DoD item does not cite it fails outright with no lexical
# fallback (the IMP-027 SCOPE-8 decision — when a stable id exists the linkage
# is a fact, not something a tunable word-overlap proxy may override).
# Scenarios that carry no id keep the lexical behavior exactly.
#
# traceability-guard.sh consumes the SAME function with the `id-hint-lenient`
# policy. That difference is now a declared argument rather than two copies
# drifting apart, and scenario-match-lib-selftest.sh asserts the two policies
# differ ONLY where documented.
# =============================================================================
echo "--- Check 22: DoD-Gherkin Content Fidelity (Gate G068) ---"

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
  # BUG-026: shared DoD parser (correct tiered-DoD boundary; case-insensitive
  # header match). Emits checkbox item text, one per line.
  scope_dod_items="$(dod_section_parse "$scope_path" | awk -F'\t' '
    $1 == "CHECKBOX" { out = $4; for (i = 5; i <= NF; i++) out = out "\t" $i; print out }
  ' || true)"

  if [[ -z "$scope_dod_items" ]]; then
    continue
  fi

  while IFS= read -r scenario; do
    [[ -n "$scenario" ]] || continue
    dod_fidelity_total=$((dod_fidelity_total + 1))

    matched=0
    while IFS= read -r dod_item; do
      [[ -n "$dod_item" ]] || continue
      if bubbles_scenario_matches_dod "$scenario" "$dod_item" structural-strict; then
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
# CHECK 43: Evidence Receipt Staleness (IMP-027 SCOPE-3, EV-2)
# =============================================================================
# Markdown evidence has one property it can never have: freshness. A pasted
# terminal block proves a command ran ONCE, against SOME version of the tree,
# and nothing in the artifact records which. Receipts written by tool-log.sh
# DO record it — each carries an `inputClosure` of the files the evidence
# depended on, hashed at capture time.
#
# evidence-receipt-check.sh already knows how to compare those hashes against
# the working tree, but until now it was reachable ONLY from its own selftest,
# so no transition ever consulted it. That made the receipt rail decorative:
# a spec could carry receipts captured before the very change under review and
# certify anyway.
#
# This check consults it on the transition path. It is deliberately
# NO-OP-UNLESS-EARNED:
#   - no tool log                     -> skipped (the overwhelming majority)
#   - receipts present, none stale    -> passes
#   - receipts present, some stale    -> FAILS, naming them
#   - checker unavailable/errors      -> INFO, never blocks
# A project that never adopts receipts is unaffected; a project that adopts
# them cannot then certify against evidence its own recorded inputs invalidate.
echo "--- Check 43: Evidence Receipt Staleness (IMP-027 SCOPE-3) ---"
c43_repo_root="$(cd "$feature_dir" && git rev-parse --show-toplevel 2>/dev/null || pwd)"
c43_log="$c43_repo_root/.specify/runtime/tool-calls.jsonl"
c43_checker="$SCRIPT_DIR/evidence-receipt-check.sh"
if [[ ! -f "$c43_log" ]]; then
  info "No tool-call receipt log at .specify/runtime/tool-calls.jsonl; receipt staleness not applicable (markdown evidence rail)"
elif [[ ! -x "$c43_checker" && ! -f "$c43_checker" ]]; then
  info "evidence-receipt-check.sh not present; skipping receipt staleness"
else
  c43_out=""
  c43_rc=0
  c43_out="$(bash "$c43_checker" --log "$c43_log" --repo-root "$c43_repo_root" --strict 2>&1)" || c43_rc=$?
  case "$c43_rc" in
    0)
      pass "Evidence receipts consulted; no stale receipt backs this transition"
      ;;
    1)
      fail "Evidence receipt(s) are STALE — an input file changed after the evidence was captured, so the recorded result no longer describes the current tree. Re-run the affected command(s) to refresh the receipt. Detail: $(printf '%s' "$c43_out" | tr '\n' ' ' | head -c 400)"
      ;;
    *)
      info "evidence-receipt-check.sh could not produce a report (exit $c43_rc); receipt staleness not evaluated this run"
      ;;
  esac

  # IMP-027 SCOPE-8 (EV-3): clone detection by receipt hash plus execution
  # identity, not text similarity alone.
  #
  # Check 20 (G021) answers "is this evidence a copy of that evidence?" with an
  # 80%-similarity score over prose. That is a proxy: legitimately similar
  # output (two runs of the same suite) scores high, and a lightly-edited
  # forgery scores low. Receipts carry stdoutHash, which turns the same question
  # into an exact comparison.
  #
  # The rule is deliberately narrow. Identical output from a RE-RUN of the same
  # command is normal. Deterministic validators can also emit identical output
  # when the same validator category runs independently over distinct targets.
  # That sibling case is accepted only when family, category, and exit status
  # agree while target/input closure and execution provenance are all present
  # and distinct. A substantive collision across incompatible command families
  # or categories still identifies one result backing unrelated claims.
  if command -v jq >/dev/null 2>&1; then
    # An EMPTY stdout is excluded, and that exclusion is what makes the rule
    # correct rather than merely narrow. Every command that writes nothing to
    # stdout hashes to e3b0c442… — the SHA-256 of the empty string — so a
    # `grep` with no match, a run that wrote only to stderr, and a `--help`
    # that exited 127 all collide with each other. Reading that collision as
    # forgery accuses honest work of the single most serious thing this guard
    # can allege. A receipt with no stdout also has no evidentiary content to
    # clone, so excluding it removes the false-positive class without weakening
    # the check: a real forgery reuses a SUBSTANTIVE captured result, which is
    # by definition non-empty.
    #
    # The DIGEST is the discriminator, not `stdoutBytes`. `stdoutBytes` is
    # optional, so keying the exemption on it meant an absent field defaulted
    # to 0 and silently excluded a genuine clone from detection (BUG-007's
    # first fix over-corrected exactly this way). Every receipt of every
    # vintage carries a stdoutHash, so the empty-string digest identifies
    # empty stdout with no field required. An explicitly-present
    # `stdoutBytes: 0` is still honoured; an ABSENT one exempts nothing.
    # "SAME command" is command IDENTITY, not a byte-identical argv string.
    # Keying it on the raw string re-broke the very re-run case the rule above
    # promises never to fire on, because an honest re-run is routinely spelled
    # differently: `--repo-root .` and `--repo-root /abs/path` name one
    # directory, and an optional trailing filter argument narrows a run without
    # making it a different claim. Both produce identical stdout because they
    # ARE the same command, and the raw-string rule reported that as forgery.
    #
    # Identity is therefore (executable basename, first positional argument):
    # the tool, and the subject it ran against. Option flags and their values
    # are dropped, so a re-spelled invocation collapses to one identity, while
    # `cargo test` vs `npm run lint` — different tool, different subject — stay
    # distinct and still block. Dropping option VALUES does mean two runs of
    # one tool over one subject under different flags no longer collide; that
    # is the intended trade, because a false CLONE accuses honest work of the
    # most serious thing this guard can allege, and the surviving rule still
    # catches the case G021 exists for: one captured result reused for an
    # UNRELATED claim.
    #
    # BUG-028 fix A: a differing CATEGORY no longer TRIGGERS the allegation.
    # `evidence_category` is derived from operator-supplied `tags`, which are
    # metadata ABOUT a run, not a statement of WHAT ran. Two honest executions
    # of one command — identical cmd, identical family, identical identity —
    # two days apart in two sessions were accused of forgery solely because one
    # was tagged `test` and the other `validate`. Category survives INSIDE
    # `deterministic_siblings`, where it still constrains the multi-identity
    # case that is now the only reachable one; it just cannot, alone, allege
    # forgery when command identity is single.
    c43_empty_stdout_sha256="e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
    c43_analysis="$(jq -rs --arg empty_sha "$c43_empty_stdout_sha256" '
      # BUG-033 facet 2: unwrap every TRANSPARENT prefix, not just a bare
      # leading `bash`/`sh`. A shell invoked with `-c`, an `env` prefix, and
      # leading `VAR=value` assignments do not change WHICH program ran, so
      # three ordinary spellings of one command must resolve to one family.
      # Before this, `node -e x`, `env P=1 node -e x` and `zsh -c node -e x`
      # resolved to `node`, `env` and `zsh`, and the group was refused as a
      # multi-identity collision — the re-spelling case the rule above promises
      # to tolerate. `bash -c x` was worse still: it stripped `bash` and left
      # `-c`, so the family was a flag. The recursion is what makes composed
      # prefixes (`env A=1 zsh -c ...`) collapse rather than half-collapse.
      def strip_wrappers:
        if ((.[0] // "") | test("^(bash|sh|zsh|ksh|dash)$"))
          then (if ((.[1] // "") == "-c") then (.[2:] | strip_wrappers) else (.[1:] | strip_wrappers) end)
        elif ((.[0] // "") == "env") then (.[1:] | strip_wrappers)
        elif ((.[0] // "") | test("^[A-Za-z_][A-Za-z0-9_]*=")) then (.[1:] | strip_wrappers)
        else . end;
      def cmd_parts:
        ( . / " " | map(select(length > 0)) ) | strip_wrappers;
      def command_family:
        cmd_parts as $t
        | ( ($t[0] // "") | split("/") | last ) as $exe
        | $exe;
      # BUG-028 fix C: a generic dispatch verb is not a subject. Keeping only
      # the FIRST positional made `npm run lint` and `npm run test` one
      # identity, discarding the script name that is the actual claim, so one
      # receipt covering both escaped detection. When the first positional is
      # `run` or `exec`, the next one is retained; no wider verb list is used.
      def positional_target:
        cmd_parts as $t
        | ( reduce ($t[1:][]) as $tok ({skip:false, pos:[]};
              if .skip then {skip:false, pos:.pos}
              elif ($tok | startswith("-"))
                then {skip: (($tok | contains("=")) | not), pos:.pos}
              else {skip:false, pos:(.pos + [$tok])}
              end)
            | .pos ) as $pos
        | ($pos[0] // "") as $first
        | if ($first | test("^(run|exec)$")) and (($pos | length) > 1)
            then $first + " " + $pos[1]
            else $first end;
      def cmd_identity:
        [command_family, positional_target] | map(select(length > 0)) | join(" ");
      # BUG-028 fix D: the PROGRAM that ran, without its plain subject. One
      # deterministic validator over several subjects must resolve to one
      # program (`artifact-lint.sh`), while a dispatched script name is part of
      # the program itself (`npm run lint` vs `npm run test`). The dispatch-verb
      # decision is reused from `positional_target` rather than restated, so the
      # two definitions can never disagree about what a subject is.
      def program_identity:
        positional_target as $pt
        | if ($pt | test("^(run|exec) ")) then command_family + " " + $pt else command_family end;
      def tag_category:
        [(.tags // [])[]?
          | ascii_downcase
          | if test("^(test|unit|functional|integration|e2e|e2e-api|e2e-ui|stress|load)$") then "test"
            elif test("^(lint|clippy|fmt|format|shellcheck)$") then "lint"
            elif test("^(build|compile)$") then "build"
            elif test("^(validate|validation|guard|audit)$") then "validate"
            else empty end]
        | unique
        | if length == 1 then .[0]
          elif length > 1 then "mixed:" + join(",")
          else "" end;
      def command_category:
        ascii_downcase
        | if test("(^|[ /_-])(test|pytest|jest|vitest|playwright|e2e|stress|load)([ /_.-]|$)") then "test"
          elif test("(^|[ /_-])(lint|clippy|fmt|format|shellcheck)([ /_.-]|$)") then "lint"
          elif test("(^|[ /_-])(build|compile)([ /_.-]|$)") then "build"
          elif test("(^|[ /_-])(validate|validation|guard|audit|check)([ /_.-]|$)") then "validate"
          else "other" end;
      def evidence_category:
        tag_category as $tag
        | if $tag != "" then $tag else (.cmd | command_category) end;
      def target_identity:
        if ((.inputClosure // []) | type) == "array" and ((.inputClosure // []) | length) > 0 then
          "closure:" + ([.inputClosure[] | ((.path // "") + "@" + (.sha256 // ""))] | sort | join(","))
        elif ((.spec // "") != "") or ((.scope // "") != "") then
          # BUG-028 fix E: a spec/scope pair names WHERE the work sits, not WHAT
          # was examined. Two runs of one validator over two different files in
          # one scope collapsed to a single target and were refused for it, so
          # the subject is carried alongside. This only ever splits targets
          # further apart; it can never merge two that were distinct.
          "spec:" + (.spec // "") + "|scope:" + (.scope // "") + "|subject:" + (.cmd | positional_target)
        else
          "target:" + (.cmd | positional_target)
        end;
      def provenance_identity:
        if ((.sessionId // "") != "") or ((.ts // "") != "") then
          "session:" + (.sessionId // "") + "|ts:" + (.ts // "") + "|duration:" + ((.durationMs // "") | tostring)
        else "" end;
      def all_distinct_nonempty:
        length > 1 and all(.[]; length > 0) and ((unique | length) == length);
      def deterministic_siblings:
        . as $rows
        | ($rows | map(.cmd | command_family)) as $families
        | ($rows | map(.cmd | program_identity)) as $programs
        | ($rows | map(evidence_category)) as $categories
        | ($rows | map(.exitCode)) as $exits
        # BUG-033 facet 1: one target per command IDENTITY, not per RECEIPT.
        # A validator is routinely re-run over one subject, so an honest log
        # repeats that subject; measuring distinctness per receipt failed on
        # SHAPE ALONE before any question of forgery was asked, and refused
        # nine honest re-runs as forged evidence. Provenance distinctness below
        # is still measured PER RECEIPT, so every receipt must still prove
        # independent execution, and two identities sharing one target remain a
        # refusal because the grouped list still carries a duplicate.
        | ($rows | group_by(.cmd | cmd_identity) | map(.[0] | target_identity)) as $targets
        | ($rows | map(provenance_identity)) as $provenance
        | (($families | unique | length) == 1)
          and (($families[0] // "") != "")
          # BUG-028 fix D: a tag describes a run, it does not name the program.
          # One deterministic validator over several subjects is expected to
          # repeat its output, so identity is judged by the program that ran
          # plus distinct subjects and distinct executions — not by how an agent
          # labelled the run. Category survives only as a sanity floor applied
          # to every row: an unclassifiable or self-contradictory tagging still
          # blocks, but honest runs may legitimately carry different labels.
          and (($programs | unique | length) == 1)
          and (($programs[0] // "") != "")
          and all($categories[]; . != "other" and ((startswith("mixed:")) | not))
          and all($exits[]; type == "number")
          and (($exits | unique | length) == 1)
          and ($targets | all_distinct_nonempty)
          and ($provenance | all_distinct_nonempty);
      def identity_detail:
        "family=" + (.cmd | command_family)
        + " category=" + evidence_category
        + " target=" + target_identity
        + " provenance=" + provenance_identity
        + " cmd=" + .cmd;
      map(select((.stdoutHash // "") != "" and (.cmd // "") != "" and (.stdoutHash != $empty_sha) and ((has("stdoutBytes") and .stdoutBytes == 0) | not)))
      | group_by(.stdoutHash)
      | {
          siblings: map(select(
            ((map(.cmd | cmd_identity) | unique | length) > 1)
            and deterministic_siblings
          )),
          clones: (map(select(
            ((map(.cmd | cmd_identity) | unique | length) > 1)
            and (deterministic_siblings | not)
          )) | map({hash: .[0].stdoutHash, identities: map(identity_detail)}))
        }
    ' "$c43_log" 2>/dev/null || true)"
    c43_sibling_count="$(printf '%s' "$c43_analysis" | jq -r '.siblings | length' 2>/dev/null || echo 0)"
    c43_clones="$(printf '%s' "$c43_analysis" | jq -r '
      .clones[]?
      | "\(.hash[0:12])… reused across incompatible or unproven identities: \(.identities | join(" AND "))"
    ' 2>/dev/null || true)"
    if [[ -n "$c43_clones" ]]; then
      fail "Evidence receipt CLONE — one substantive stdout is cited across incompatible command/category identities or receipts that cannot prove independent target/execution provenance: $(printf '%s' "$c43_clones" | tr '\n' ';' | head -c 800)"
    elif [[ "$c43_sibling_count" -gt 0 ]]; then
      pass "No receipt clones ($c43_sibling_count deterministic sibling hash collision(s) accepted by compatible family/category/exit plus distinct target and execution provenance)"
    else
      pass "No receipt clones (no substantive stdout hash shared across incompatible or unproven receipt identities)"
    fi
  fi
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
# fixture specs. In downstream/fixture repositories, G085-CURRENT-DONE
# passes on current numbered state evidence with exact top-level `status:
# done`. G085-FIRST-ADOPTION passes only when the required current-state
# and complete-history evidence is proven; missing or incomplete evidence
# fails closed.
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
# CHECK 40: Claim-Source provenance (IMP-101 SCOPE-1 / gate G072)
# Delegates to the standalone claim-source-lint.sh in an ISOLATED subprocess —
# its own `set -e` can never abort this guard. Advisory-until-opt-in: the lint
# exits non-zero ONLY when `claimSourceProvenanceGuard: block` is set in
# .github/bubbles-project.yaml, so a transition is failed here only when the
# operator has explicitly opted in. Otherwise findings print but do not block.
# =============================================================================
if [[ -x "$SCRIPT_DIR/claim-source-lint.sh" ]]; then
  echo "--- Check 40: Claim-Source provenance (G072) ---"
  if bash "$SCRIPT_DIR/claim-source-lint.sh" "$feature_dir"; then
    pass "Claim-Source provenance: execution-evidence blocks carry a valid tag (or advisory)"
  else
    fail "Claim-Source provenance findings under claimSourceProvenanceGuard: block (G072)"
  fi
  echo ""
fi

# =============================================================================
# CHECK 44: Plan Dependency Depth (IMP-031 SCOPE-8 / IMP-022 SCOPE-3 + SCOPE-4)
# CHECK 45: Release Assurance deploy-eligibility (IMP-031 SCOPE-8 / IMP-100 P3)
# =============================================================================
# Both scripts shipped complete, with selftests wired into framework-validate,
# and NO production caller. A selftest proves a guard CAN detect something; it
# never lets the guard detect anything. Until this wiring they could not fail a
# real transition, so their green selftests were assurance about nothing.
#
# Wired straight through rather than wrapped in a new report-only knob: each
# script already owns its own posture and no-op rules, and re-deciding them here
# would fork the contract.
#   - plan-dependency-depth-guard.sh exits 1 ONLY under an operator-selected
#     block posture; otherwise findings print advisory and it exits 0.
#   - release-assurance-gate.sh no-ops without config/release-trains.yaml, skips
#     without yq, and skips any spec lacking certification.assurance.level or
#     releaseTrain. It exits 1 only for a certified feature whose achieved
#     assurance is below its target train's floor — a real deploy-eligibility
#     breach the operator explicitly configured a floor to catch.
# Each runs in its own subprocess so its `set -e` cannot abort this guard.
if [[ -x "$SCRIPT_DIR/plan-dependency-depth-guard.sh" ]]; then
  echo "--- Check 44: Plan Dependency Depth (horizontal-layer DAG analysis) ---"
  if fixture_gate_skip "plan dependency depth"; then
    :
  elif bash "$SCRIPT_DIR/plan-dependency-depth-guard.sh" "$feature_dir"; then
    pass "Plan dependency depth: no blocking horizontal-plan violation"
  else
    fail "Plan dependency depth violation under block posture (every consumer-visible scope sits behind >=3 foundation scopes)"
  fi
  echo ""
fi

if [[ -x "$SCRIPT_DIR/release-assurance-gate.sh" ]]; then
  echo "--- Check 45: Release Assurance deploy-eligibility ---"
  if fixture_gate_skip "release assurance"; then
    :
  elif bash "$SCRIPT_DIR/release-assurance-gate.sh" "$guard_repo_root"; then
    pass "Release assurance: no certified feature targets a train above its achieved assurance"
  else
    fail "Release assurance breach: a certified feature's achieved assurance is below its target train's minimum floor"
  fi
  echo ""
fi

# --------------------------------------------------------------------------
# Check 46 (IMP-031 SCOPE-6): run the vertical-delivery plan guard for real.
#
# The guard has shipped since IMP-022 with a 13-case selftest and no production
# caller, so the only plan it has ever classified is a fixture. A guard that
# only runs its own selftest proves it CAN detect something while never being
# allowed to detect anything.
#
# This wiring adds NO blocking threshold of its own. The guard is advisory by
# construction and exits non-zero only when the repo has explicitly opted in
# with `verticalPlanGuard: block` in .github/bubbles-project.yaml, so an
# unconfigured repo can only ever see a warning here.
# --------------------------------------------------------------------------
if [[ -x "$SCRIPT_DIR/vertical-delivery-plan-guard.sh" ]]; then
  echo "--- Check 46: Vertical-delivery plan shape (horizontal chain / scope budget / per-increment exposure) ---"
  if fixture_gate_skip "vertical delivery plan"; then
    :
  elif bash "$SCRIPT_DIR/vertical-delivery-plan-guard.sh" "$feature_dir"; then
    pass "Vertical-delivery plan: no blocking plan-shape violation"
  else
    fail "Vertical-delivery plan violation under block posture (horizontal chain, low-risk scope budget, or an increment with no consumer surface and no declared deferral)"
  fi
  echo ""
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

  if [[ "$revert_on_fail" == "true" \
    && "$transition_audit_profile" == "delivery-completion-v1" \
    && -f "$state_file" ]]; then
    echo "--- Auto-Reverting state.json (--revert-on-fail) ---"
    now_utc="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    revert_tmp="$(mktemp "${TMPDIR:-/tmp}/bubbles-transition-revert.XXXXXX")"
    if ! jq \
      --arg now "$now_utc" \
      --arg summary "$failures blocking failures detected by state-transition-guard.sh" '
      def clear_completion_arrays:
        if type == "object" then
          with_entries(
            if (.key == "completedScopes"
              or .key == "certifiedCompletedPhases"
              or .key == "completedPhaseClaims"
              or .key == "completedPhases") then
              .value = []
            else
              .value |= clear_completion_arrays
            end
          )
        elif type == "array" then
          map(clear_completion_arrays)
        else
          .
        end;
      clear_completion_arrays
      | .status = "in_progress"
      | if (.certification | type) == "object" then .certification.status = "in_progress" else . end
      | if has("lastUpdatedAt") then .lastUpdatedAt = $now else . end
      | if (.failures | type) == "array" then
          .failures = ([{
            phase: "transition-guard",
            summary: $summary,
            detectedAt: $now
          }] + .failures)
        else . end
    ' "$state_file" > "$revert_tmp"; then
      rm -f "$revert_tmp"
      fail "--revert-on-fail could not rewrite state.json atomically"
    else
      mv "$revert_tmp" "$state_file"
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

  if [[ "$revert_on_fail" == "true" && "$transition_audit_profile" != "delivery-completion-v1" ]]; then
    info "--revert-on-fail is delivery-only; planning state was not rewritten"
  fi

  if [[ ${#failed_check_ids[@]} -eq 0 && ${#failed_gate_ids[@]} -eq 0 ]]; then
    record_failed_check applicable-integrity
  fi
  transition_blocking_code="DELIVERY_COMPLETION_FAILED"
  if [[ "$transition_audit_profile" == "planning-maturity-v1" ]]; then
    transition_blocking_code="PLANNING_GATE_FAILED"
    if list_contains G073 ${failed_gate_ids[@]+"${failed_gate_ids[@]}"}; then
      transition_blocking_code="SOURCE_EDIT_LOCKOUT"
    fi
  fi
  emit_transition_result FAIL "$transition_blocking_code" "$failures" 1
  exit 1
else
  if [[ "$warnings" -gt 0 ]]; then
    echo "🟡 TRANSITION PERMITTED with $warnings warning(s)"
  else
    echo "🟢 TRANSITION PERMITTED: All checks pass ($failures failures, $warnings warnings)"
    fun_summary pass
  fi
  echo ""
  final_status_ceiling="$transition_target_status"
  if [[ -n "$final_status_ceiling" && "$state_status" == "$final_status_ceiling" && "$final_status_ceiling" != "done" ]]; then
    echo "state.json is correctly set to '$state_status' for workflowMode '$state_workflow_mode'."
  elif [[ "$final_status_ceiling" == "done" ]]; then
    echo "state.json status may be set to 'done'."
  else
    echo "state.json status '$state_status' is permitted for workflowMode '$state_workflow_mode'."
  fi
  emit_transition_result PASS none 0 0
  exit 0
fi
