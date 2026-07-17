#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD_LIB="$SCRIPT_DIR/guard-lib.sh"

if [[ ! -f "$GUARD_LIB" ]]; then
  printf 'audit-result-contract-lint: FAIL [DEPENDENCY]: guard-lib.sh is unavailable\n' >&2
  exit 2
fi

# shellcheck source=/dev/null
source "$GUARD_LIB"

LC_ALL=C
export LC_ALL

AUDIT_FIELDS=(
  schemaVersion
  runId
  attemptId
  target
  targetRevision
  workflowMode
  modeClass
  auditClass
  statusCeiling
  requestedStatus
  auditVerdict
  outcome
  resultState
  certifiedStatus
  planningEvaluation
  deliveryEvaluation
  sourceEditLockout
  applicableCheckClasses
  notApplicableChecks
  passedGateIds
  failedGateIds
  failedChecks
  blockingCode
  unresolvedFields
  contradictions
  contractRef
  contractDigest
  evidenceRefs
  addressedFindings
  unresolvedFindings
  nextRequiredOwner
  supersedesAttemptId
  resumeFromPhase
)

TRANSITION_FIELDS=(
  schemaVersion
  workflowMode
  auditProfile
  targetStatus
  contractDigest
  targetRevision
  applicableCheckClasses
  notApplicableChecks
  passedGateIds
  failedGateIds
  failedChecks
  blockingCode
  failureCount
  exitStatus
  verdict
)

usage() {
  cat <<'EOF'
Usage:
  bash bubbles/scripts/audit-result-contract-lint.sh --result FILE
  bash bubbles/scripts/audit-result-contract-lint.sh --agent-contract FILE
EOF
}

fail() {
  local code="$1"
  shift
  printf 'audit-result-contract-lint: FAIL [%s]: %s\n' "$code" "$*" >&2
  exit 1
}

marker_count() {
  local file="$1"
  local marker="$2"
  awk -v marker="$marker" '$0 == marker { count++ } END { print count + 0 }' "$file"
}

extract_block() {
  local file="$1"
  local begin_marker="$2"
  local end_marker="$3"
  local output_file="$4"

  awk -v begin_marker="$begin_marker" -v end_marker="$end_marker" '
    $0 == begin_marker { active=1 }
    active { print }
    $0 == end_marker && active { exit }
  ' "$file" > "$output_file"
}

field_value() {
  local block_file="$1"
  local field="$2"
  awk -v prefix="$field: " 'index($0, prefix) == 1 { print substr($0, length(prefix) + 1); exit }' "$block_file"
}

validate_ordered_block() {
  local block_file="$1"
  local begin_marker="$2"
  local end_marker="$3"
  local field_set="$4"
  local expected_lines
  local actual_lines
  local line_number=0
  local line=""
  local field_index
  local expected_field

  if [[ "$field_set" == "audit" ]]; then
    expected_lines=$((${#AUDIT_FIELDS[@]} + 2))
  else
    expected_lines=$((${#TRANSITION_FIELDS[@]} + 2))
  fi
  actual_lines="$(awk 'END { print NR + 0 }' "$block_file")"
  [[ "$actual_lines" -eq "$expected_lines" ]] || fail SCHEMA "${field_set} block has ${actual_lines} lines; expected ${expected_lines}"

  while IFS= read -r line || [[ -n "$line" ]]; do
    line_number=$((line_number + 1))
    if [[ "$line_number" -eq 1 ]]; then
      [[ "$line" == "$begin_marker" ]] || fail SCHEMA "${field_set} begin marker is malformed"
      continue
    fi
    if [[ "$line_number" -eq "$expected_lines" ]]; then
      [[ "$line" == "$end_marker" ]] || fail SCHEMA "${field_set} end marker is malformed"
      continue
    fi

    field_index=$((line_number - 2))
    if [[ "$field_set" == "audit" ]]; then
      expected_field="${AUDIT_FIELDS[$field_index]}"
    else
      expected_field="${TRANSITION_FIELDS[$field_index]}"
    fi
    [[ "$line" == "$expected_field: "* ]] || fail SCHEMA "${field_set} field ${line_number} must be '${expected_field}'"
    [[ "$line" != "$expected_field: " ]] || fail SCHEMA "${field_set}.${expected_field} must not be empty"
  done < "$block_file"
}

validate_ascii_file() {
  local file="$1"
  if LC_ALL=C grep -n '[^ -~]' "$file" >/dev/null 2>&1; then
    fail PRESENTATION "result contains non-ASCII, control, color, or carriage-return bytes"
  fi
}

validate_token_list() {
  local name="$1"
  local value="$2"
  local inner
  local item
  local seen=","
  local old_ifs

  [[ "$value" == \[*\] ]] || fail SCHEMA "$name must use [comma,separated] collection syntax"
  inner="${value#\[}"
  inner="${inner%\]}"
  [[ -n "$inner" ]] || return 0
  [[ "$inner" != *, ]] || fail SCHEMA "$name has a trailing comma"
  [[ "$inner" != ,* ]] || fail SCHEMA "$name has a leading comma"
  [[ "$inner" != *',,'* ]] || fail SCHEMA "$name has an empty item"

  old_ifs="$IFS"
  IFS=','
  for item in $inner; do
    [[ "$item" =~ ^[-A-Za-z0-9._/@#:=+]+$ ]] || fail SCHEMA "$name contains malformed item '$item'"
    case "$seen" in
      *",$item,"*) fail SCHEMA "$name contains duplicate item '$item'" ;;
    esac
    seen="$seen$item,"
  done
  IFS="$old_ifs"
}

list_contains() {
  local value="$1"
  local needle="$2"
  local inner="${value#\[}"
  inner="${inner%\]}"
  case ",$inner," in
    *",$needle,"*) return 0 ;;
  esac
  return 1
}

list_has_prefix() {
  local value="$1"
  local prefix="$2"
  local inner="${value#\[}"
  local item
  local old_ifs
  inner="${inner%\]}"
  old_ifs="$IFS"
  IFS=','
  for item in $inner; do
    if [[ "$item" == "$prefix"* ]]; then
      IFS="$old_ifs"
      return 0
    fi
  done
  IFS="$old_ifs"
  return 1
}

assert_equal() {
  local name="$1"
  local observed="$2"
  local expected="$3"
  [[ "$observed" == "$expected" ]] || fail CONSISTENCY "$name mismatch: observed '$observed', expected '$expected'"
}

assert_collection_disjoint() {
  local left_name="$1"
  local left="$2"
  local right_name="$3"
  local right="$4"
  local inner="${left#\[}"
  local item
  local old_ifs
  inner="${inner%\]}"
  old_ifs="$IFS"
  IFS=','
  for item in $inner; do
    if [[ -n "$item" ]] && list_contains "$right" "$item"; then
      IFS="$old_ifs"
      fail FINDINGS "$item appears in both $left_name and $right_name"
    fi
  done
  IFS="$old_ifs"
}

expected_profile_for_class() {
  case "$1" in
    planning-maturity) printf '%s\n' 'planning-maturity-v1' ;;
    delivery-completion) printf '%s\n' 'delivery-completion-v1' ;;
    UNRESOLVED) printf '%s\n' 'UNRESOLVED' ;;
    *) fail ENUM "unsupported auditClass '$1'" ;;
  esac
}

validate_agent_contract() {
  local agent_file="$1"
  local block_file
  local preflight
  local token

  [[ -f "$agent_file" ]] || fail INPUT "agent contract does not exist: $agent_file"
  [[ "$(marker_count "$agent_file" 'BEGIN AUDIT_RESULT_V1')" -eq 1 ]] || fail AGENT_SCHEMA 'agent contract must contain exactly one AUDIT_RESULT_V1 begin marker'
  [[ "$(marker_count "$agent_file" 'END AUDIT_RESULT_V1')" -eq 1 ]] || fail AGENT_SCHEMA 'agent contract must contain exactly one AUDIT_RESULT_V1 end marker'
  block_file="$(mktemp "${TMPDIR:-/tmp}/bubbles-audit-agent-block.XXXXXXXX")"
  trap 'rm -f "$block_file"' EXIT INT TERM
  extract_block "$agent_file" 'BEGIN AUDIT_RESULT_V1' 'END AUDIT_RESULT_V1' "$block_file"
  validate_ordered_block "$block_file" 'BEGIN AUDIT_RESULT_V1' 'END AUDIT_RESULT_V1' audit

  preflight="$(awk '
    /^### 0-pre\. State Transition Guard/ { active=1 }
    active && /^### 0\. / { exit }
    active { print }
  ' "$agent_file")"
  [[ -n "$preflight" ]] || fail AGENT_PREFLIGHT 'Audit 0-pre section is missing'
  for token in \
    'bubbles/scripts/transition-contract-resolver.sh' \
    'bubbles/scripts/state-transition-guard.sh' \
    '--target-status' \
    '--expect-workflow-mode' \
    '--expect-contract-digest' \
    'TRANSITION_GUARD_RESULT_V1' \
    'assertion-only'; do
    [[ "$preflight" == *"$token"* ]] || fail AGENT_PREFLIGHT "Audit 0-pre is missing '$token'"
  done
  [[ "$preflight" != *'--revert-on-fail'* ]] || fail AGENT_AUTHORITY 'Audit 0-pre must not mutate state through --revert-on-fail'

  for token in \
    'audit-run/v1' \
    'currentAttemptId' \
    'SUPERSEDED' \
    'INCOMPLETE' \
    'one ACTIVE' \
    'PLANNING_AUDIT_CLEAN' \
    'PLANNING_REWORK_REQUIRED' \
    'SHIP_IT' \
    'DO_NOT_SHIP' \
    'NOT_APPLICABLE' \
    "Audit MUST NOT write \`certification.*\`" \
    'Audit MUST NOT mark scopes Done' \
    'Audit MUST NOT check DoD items'; do
    grep -Fq -- "$token" "$agent_file" || fail AGENT_CONTRACT "agent contract is missing '$token'"
  done

  rm -f "$block_file"
  trap - EXIT INT TERM
  printf 'audit-result-contract-lint: PASS agent-contract %s\n' "$agent_file"
}

validate_result_contract() {
  local result_file="$1"
  local audit_block
  local transition_block
  local human_block
  local state_file
  local target_path
  local expected_profile
  local active_count
  local current_attempt
  local attempt_count
  local attempt_state
  local attempt_total
  local prior_attempt_count
  local prior_state
  local prior_finding
  local evidence_ref
  local human_verdict_count
  local human_verdict
  local human_value
  local not_applicable_inner
  local not_applicable_item
  local human_line
  local schema_version run_id attempt_id target target_revision workflow_mode mode_class audit_class
  local status_ceiling requested_status audit_verdict outcome result_state certified_status
  local planning_evaluation delivery_evaluation source_edit_lockout applicable_check_classes
  local not_applicable_checks passed_gate_ids failed_gate_ids failed_checks blocking_code
  local unresolved_fields contradictions contract_ref contract_digest evidence_refs
  local addressed_findings unresolved_findings next_required_owner supersedes_attempt_id resume_from_phase
  local guard_schema guard_workflow_mode guard_profile guard_target_status guard_digest guard_revision
  local guard_classes guard_not_applicable guard_passed_gates guard_failed_gates guard_failed_checks
  local guard_blocking_code guard_failure_count guard_exit_status guard_verdict
  local human_key expected_human observed_human

  [[ -f "$result_file" ]] || fail INPUT "result file does not exist: $result_file"
  command -v jq >/dev/null 2>&1 || fail DEPENDENCY 'jq is required to validate persisted audit attempts'
  validate_ascii_file "$result_file"

  [[ "$(marker_count "$result_file" 'BEGIN AUDIT_RESULT_V1')" -eq 1 ]] || fail SCHEMA 'result must contain exactly one AUDIT_RESULT_V1 begin marker'
  [[ "$(marker_count "$result_file" 'END AUDIT_RESULT_V1')" -eq 1 ]] || fail SCHEMA 'result must contain exactly one AUDIT_RESULT_V1 end marker'
  [[ "$(marker_count "$result_file" 'BEGIN TRANSITION_GUARD_RESULT_V1')" -eq 1 ]] || fail GUARD_SCHEMA 'result must contain exactly one TRANSITION_GUARD_RESULT_V1 begin marker'
  [[ "$(marker_count "$result_file" 'END TRANSITION_GUARD_RESULT_V1')" -eq 1 ]] || fail GUARD_SCHEMA 'result must contain exactly one TRANSITION_GUARD_RESULT_V1 end marker'

  audit_block="$(mktemp "${TMPDIR:-/tmp}/bubbles-audit-result.XXXXXXXX")"
  transition_block="$(mktemp "${TMPDIR:-/tmp}/bubbles-transition-result.XXXXXXXX")"
  human_block="$(mktemp "${TMPDIR:-/tmp}/bubbles-audit-human.XXXXXXXX")"
  trap 'rm -f "$audit_block" "$transition_block" "$human_block"' EXIT INT TERM
  extract_block "$result_file" 'BEGIN AUDIT_RESULT_V1' 'END AUDIT_RESULT_V1' "$audit_block"
  extract_block "$result_file" 'BEGIN TRANSITION_GUARD_RESULT_V1' 'END TRANSITION_GUARD_RESULT_V1' "$transition_block"
  awk '
    $0 == "END TRANSITION_GUARD_RESULT_V1" { active=1; next }
    $0 == "BEGIN AUDIT_RESULT_V1" { exit }
    active { print }
  ' "$result_file" > "$human_block"

  validate_ordered_block "$audit_block" 'BEGIN AUDIT_RESULT_V1' 'END AUDIT_RESULT_V1' audit
  validate_ordered_block "$transition_block" 'BEGIN TRANSITION_GUARD_RESULT_V1' 'END TRANSITION_GUARD_RESULT_V1' transition

  schema_version="$(field_value "$audit_block" schemaVersion)"
  run_id="$(field_value "$audit_block" runId)"
  attempt_id="$(field_value "$audit_block" attemptId)"
  target="$(field_value "$audit_block" target)"
  target_revision="$(field_value "$audit_block" targetRevision)"
  workflow_mode="$(field_value "$audit_block" workflowMode)"
  mode_class="$(field_value "$audit_block" modeClass)"
  audit_class="$(field_value "$audit_block" auditClass)"
  status_ceiling="$(field_value "$audit_block" statusCeiling)"
  requested_status="$(field_value "$audit_block" requestedStatus)"
  audit_verdict="$(field_value "$audit_block" auditVerdict)"
  outcome="$(field_value "$audit_block" outcome)"
  result_state="$(field_value "$audit_block" resultState)"
  certified_status="$(field_value "$audit_block" certifiedStatus)"
  planning_evaluation="$(field_value "$audit_block" planningEvaluation)"
  delivery_evaluation="$(field_value "$audit_block" deliveryEvaluation)"
  source_edit_lockout="$(field_value "$audit_block" sourceEditLockout)"
  applicable_check_classes="$(field_value "$audit_block" applicableCheckClasses)"
  not_applicable_checks="$(field_value "$audit_block" notApplicableChecks)"
  passed_gate_ids="$(field_value "$audit_block" passedGateIds)"
  failed_gate_ids="$(field_value "$audit_block" failedGateIds)"
  failed_checks="$(field_value "$audit_block" failedChecks)"
  blocking_code="$(field_value "$audit_block" blockingCode)"
  unresolved_fields="$(field_value "$audit_block" unresolvedFields)"
  contradictions="$(field_value "$audit_block" contradictions)"
  contract_ref="$(field_value "$audit_block" contractRef)"
  contract_digest="$(field_value "$audit_block" contractDigest)"
  evidence_refs="$(field_value "$audit_block" evidenceRefs)"
  addressed_findings="$(field_value "$audit_block" addressedFindings)"
  unresolved_findings="$(field_value "$audit_block" unresolvedFindings)"
  next_required_owner="$(field_value "$audit_block" nextRequiredOwner)"
  supersedes_attempt_id="$(field_value "$audit_block" supersedesAttemptId)"
  resume_from_phase="$(field_value "$audit_block" resumeFromPhase)"

  guard_schema="$(field_value "$transition_block" schemaVersion)"
  guard_workflow_mode="$(field_value "$transition_block" workflowMode)"
  guard_profile="$(field_value "$transition_block" auditProfile)"
  guard_target_status="$(field_value "$transition_block" targetStatus)"
  guard_digest="$(field_value "$transition_block" contractDigest)"
  guard_revision="$(field_value "$transition_block" targetRevision)"
  guard_classes="$(field_value "$transition_block" applicableCheckClasses)"
  guard_not_applicable="$(field_value "$transition_block" notApplicableChecks)"
  guard_passed_gates="$(field_value "$transition_block" passedGateIds)"
  guard_failed_gates="$(field_value "$transition_block" failedGateIds)"
  guard_failed_checks="$(field_value "$transition_block" failedChecks)"
  guard_blocking_code="$(field_value "$transition_block" blockingCode)"
  guard_failure_count="$(field_value "$transition_block" failureCount)"
  guard_exit_status="$(field_value "$transition_block" exitStatus)"
  guard_verdict="$(field_value "$transition_block" verdict)"

  assert_equal schemaVersion "$schema_version" 'audit-result/v1'
  assert_equal transition.schemaVersion "$guard_schema" 'transition-guard-result/v1'
  [[ "$run_id" =~ ^[-A-Za-z0-9._:]+$ ]] || fail ENUM "runId is malformed"
  [[ "$attempt_id" =~ ^[-A-Za-z0-9._:]+$ ]] || fail ENUM "attemptId is malformed"
  [[ -n "$target" && "$target" != "none" && "$target" != *'['* ]] || fail ENUM 'target is malformed'
  [[ "$workflow_mode" == "UNRESOLVED" || "$workflow_mode" =~ ^[-A-Za-z0-9._:]+$ ]] || fail ENUM 'workflowMode is malformed'
  [[ "$mode_class" == "none" || "$mode_class" == "UNRESOLVED" || "$mode_class" =~ ^[-A-Za-z0-9._:]+$ ]] || fail ENUM 'modeClass is malformed'
  [[ "$requested_status" == "none" || "$requested_status" == "UNRESOLVED" || "$requested_status" =~ ^[-A-Za-z0-9._:]+$ ]] || fail ENUM 'requestedStatus is malformed'
  case "$audit_class" in planning-maturity|delivery-completion|UNRESOLVED) ;; *) fail ENUM "invalid auditClass '$audit_class'" ;; esac
  case "$audit_verdict" in PLANNING_AUDIT_CLEAN|PLANNING_REWORK_REQUIRED|SHIP_IT|SHIP_WITH_NOTES|REWORK_REQUIRED|DO_NOT_SHIP|BLOCKED|INTERRUPTED) ;; *) fail ENUM "invalid auditVerdict '$audit_verdict'" ;; esac
  case "$outcome" in completed_diagnostic|route_required|blocked) ;; *) fail ENUM "invalid outcome '$outcome'" ;; esac
  case "$result_state" in ACTIVE|SUPERSEDED|INCOMPLETE) ;; *) fail ENUM "invalid resultState '$result_state'" ;; esac
  case "$planning_evaluation" in CERTIFIED|REWORK_REQUIRED|NOT_EVALUATED) ;; *) fail ENUM "invalid planningEvaluation '$planning_evaluation'" ;; esac
  case "$delivery_evaluation" in CERTIFIED|REFUSED|NOT_EVALUATED) ;; *) fail ENUM "invalid deliveryEvaluation '$delivery_evaluation'" ;; esac
  case "$source_edit_lockout" in PASS|FAIL|NOT_EVALUATED) ;; *) fail ENUM "invalid sourceEditLockout '$source_edit_lockout'" ;; esac
  [[ "$next_required_owner" == "none" || "$next_required_owner" =~ ^bubbles\.[a-z0-9.-]+$ ]] || fail ENUM 'nextRequiredOwner must be a concrete bubbles agent or none'
  [[ "$resume_from_phase" == "none" || "$resume_from_phase" =~ ^[1-6]$ ]] || fail ENUM 'resumeFromPhase must be none or phase 1-6'
  [[ "$supersedes_attempt_id" == "none" || "$supersedes_attempt_id" =~ ^[-A-Za-z0-9._:]+$ ]] || fail ENUM 'supersedesAttemptId is malformed'
  [[ "$guard_failure_count" =~ ^[0-9]+$ ]] || fail GUARD_SCHEMA 'guard failureCount must be numeric'
  case "$guard_exit_status:$guard_verdict" in 0:PASS|1:FAIL|2:BLOCKED) ;; *) fail GUARD_SCHEMA "guard exitStatus/verdict drift: $guard_exit_status/$guard_verdict" ;; esac
  case "$guard_verdict" in
    PASS)
      [[ "$guard_failure_count" -eq 0 && "$guard_blocking_code" == "none" ]] || fail GUARD_SCHEMA 'guard PASS must have zero failures and blockingCode none'
      ;;
    FAIL)
      [[ "$guard_failure_count" -gt 0 && "$guard_blocking_code" != "none" ]] || fail GUARD_SCHEMA 'guard FAIL must carry failures and a blocking code'
      ;;
    BLOCKED)
      [[ "$guard_failure_count" -gt 0 && "$guard_blocking_code" =~ ^E009-[A-Z0-9-]+$ ]] || fail GUARD_SCHEMA 'guard BLOCKED must carry an E009 blocking code'
      ;;
  esac

  for human_value in \
    "$applicable_check_classes" "$not_applicable_checks" "$passed_gate_ids" "$failed_gate_ids" \
    "$failed_checks" "$unresolved_fields" "$contradictions" "$evidence_refs" \
    "$addressed_findings" "$unresolved_findings" "$guard_classes" "$guard_not_applicable" \
    "$guard_passed_gates" "$guard_failed_gates" "$guard_failed_checks"; do
    validate_token_list collection "$human_value"
  done
  assert_collection_disjoint addressedFindings "$addressed_findings" unresolvedFindings "$unresolved_findings"
  assert_collection_disjoint notApplicableChecks "$not_applicable_checks" failedChecks "$failed_checks"

  expected_profile="$(expected_profile_for_class "$audit_class")"
  if [[ "$audit_class" != "UNRESOLVED" ]]; then
    assert_equal requestedStatus "$requested_status" "$status_ceiling"
  fi
  assert_equal guard.workflowMode "$guard_workflow_mode" "$workflow_mode"
  assert_equal guard.auditProfile "$guard_profile" "$expected_profile"
  assert_equal guard.targetStatus "$guard_target_status" "$status_ceiling"
  assert_equal guard.contractDigest "$guard_digest" "$contract_digest"
  assert_equal guard.targetRevision "$guard_revision" "$target_revision"
  assert_equal guard.applicableCheckClasses "$guard_classes" "$applicable_check_classes"
  assert_equal guard.notApplicableChecks "$guard_not_applicable" "$not_applicable_checks"
  assert_equal guard.passedGateIds "$guard_passed_gates" "$passed_gate_ids"
  assert_equal guard.failedGateIds "$guard_failed_gates" "$failed_gate_ids"
  assert_equal guard.failedChecks "$guard_failed_checks" "$failed_checks"

  if [[ "$audit_verdict" == "PLANNING_AUDIT_CLEAN" || "$audit_verdict" == "SHIP_IT" || "$audit_verdict" == "SHIP_WITH_NOTES" ]]; then
    [[ "$guard_verdict" == "PASS" ]] || fail CONSISTENCY 'a positive audit verdict requires guard PASS'
  fi
  if [[ "$guard_verdict" == "BLOCKED" ]]; then
    [[ "$audit_verdict" == "BLOCKED" || "$audit_verdict" == "INTERRUPTED" ]] || fail CONSISTENCY 'guard BLOCKED cannot become a maturity verdict'
  fi

  case "$audit_class:$audit_verdict" in
    planning-maturity:PLANNING_AUDIT_CLEAN)
      [[ "$outcome" == "completed_diagnostic" && "$result_state" == "ACTIVE" && "$certified_status" == "$status_ceiling" && "$planning_evaluation" == "CERTIFIED" && "$delivery_evaluation" == "NOT_EVALUATED" && "$blocking_code" == "none" && "$next_required_owner" == "none" && "$unresolved_findings" == "[]" ]] || fail VERDICT 'planning clean field combination is inconsistent'
      ;;
    planning-maturity:PLANNING_REWORK_REQUIRED)
      [[ "$outcome" == "route_required" && "$result_state" == "ACTIVE" && "$certified_status" == "none" && "$planning_evaluation" == "REWORK_REQUIRED" && "$delivery_evaluation" == "NOT_EVALUATED" && "$blocking_code" == "PLANNING_GATE_FAILED" && "$next_required_owner" != "none" && "$unresolved_findings" != "[]" ]] || fail VERDICT 'planning rework field combination is inconsistent'
      ;;
    delivery-completion:SHIP_IT|delivery-completion:SHIP_WITH_NOTES)
      [[ "$outcome" == "completed_diagnostic" && "$result_state" == "ACTIVE" && "$certified_status" == "$status_ceiling" && "$planning_evaluation" == "NOT_EVALUATED" && "$delivery_evaluation" == "CERTIFIED" && "$blocking_code" == "none" && "$next_required_owner" == "none" && "$unresolved_findings" == "[]" ]] || fail VERDICT 'delivery certification field combination is inconsistent'
      ;;
    delivery-completion:REWORK_REQUIRED|delivery-completion:DO_NOT_SHIP)
      [[ "$outcome" == "route_required" && "$result_state" == "ACTIVE" && "$certified_status" == "none" && "$planning_evaluation" == "NOT_EVALUATED" && "$delivery_evaluation" == "REFUSED" && "$blocking_code" == "DELIVERY_COMPLETION_FAILED" && "$next_required_owner" != "none" && "$unresolved_findings" != "[]" ]] || fail VERDICT 'delivery refusal field combination is inconsistent'
      ;;
    planning-maturity:BLOCKED|delivery-completion:BLOCKED|UNRESOLVED:BLOCKED)
      [[ "$outcome" == "blocked" && "$result_state" == "ACTIVE" && "$certified_status" == "none" && "$planning_evaluation" == "NOT_EVALUATED" && "$delivery_evaluation" == "NOT_EVALUATED" && "$blocking_code" != "none" && "$next_required_owner" != "none" ]] || fail VERDICT 'blocked field combination is inconsistent'
      ;;
    planning-maturity:INTERRUPTED|delivery-completion:INTERRUPTED|UNRESOLVED:INTERRUPTED)
      [[ "$outcome" == "blocked" && "$result_state" == "INCOMPLETE" && "$certified_status" == "none" && "$planning_evaluation" == "NOT_EVALUATED" && "$delivery_evaluation" == "NOT_EVALUATED" && "$blocking_code" != "none" && "$next_required_owner" == "bubbles.audit" && "$resume_from_phase" != "none" ]] || fail VERDICT 'interrupted field combination is inconsistent'
      ;;
    *) fail VERDICT "verdict '$audit_verdict' is invalid for auditClass '$audit_class'" ;;
  esac

  if [[ "$audit_class" == "planning-maturity" ]]; then
    list_contains "$applicable_check_classes" universal || fail PROFILE 'planning result is missing universal check class'
    list_contains "$applicable_check_classes" planning-maturity || fail PROFILE 'planning result is missing planning-maturity check class'
    [[ "$not_applicable_checks" != "[]" ]] || fail PROFILE 'planning result must name non-applicable delivery checks'
    for human_value in Check-4 Check-5 Check-8 Check-11; do
      list_has_prefix "$not_applicable_checks" "$human_value" || fail PROFILE "planning result omits non-applicable $human_value delivery checks"
    done
    if grep -Eiq 'SHIP_IT|SHIP_WITH_NOTES|DO_NOT_SHIP|approved for merge|merge-ready|releasable|deployable|delivered|shipped|delivery (passed|certified|approved)' "$human_block"; then
      fail VOCABULARY 'planning output contains shipment or positive delivery language'
    fi
    not_applicable_inner="${not_applicable_checks#\[}"
    not_applicable_inner="${not_applicable_inner%\]}"
    while IFS= read -r human_line || [[ -n "$human_line" ]]; do
      local old_ifs="$IFS"
      IFS=','
      for not_applicable_item in $not_applicable_inner; do
        if [[ "$human_line" == *"$not_applicable_item"* && "$human_line" =~ (PASS|CERTIFIED) ]]; then
          IFS="$old_ifs"
          fail PROFILE "$not_applicable_item is reported as PASS/CERTIFIED"
        fi
      done
      IFS="$old_ifs"
    done < "$human_block"
  elif [[ "$audit_class" == "delivery-completion" ]]; then
    list_contains "$applicable_check_classes" delivery-completion || fail PROFILE 'delivery result is missing delivery-completion check class'
    [[ "$not_applicable_checks" == "[]" ]] || fail PROFILE 'delivery result cannot use planning non-applicable checks'
    [[ "$delivery_evaluation" != "NOT_EVALUATED" ]] || fail VERDICT 'delivery verdict drifted to NOT_EVALUATED'
  fi

  if list_contains "$failed_gate_ids" G073; then
    [[ "$source_edit_lockout" == "FAIL" && "$blocking_code" == "SOURCE_EDIT_LOCKOUT" ]] || fail LOCKOUT 'failed G073 must block as SOURCE_EDIT_LOCKOUT'
  elif list_contains "$passed_gate_ids" G073; then
    [[ "$source_edit_lockout" == "PASS" ]] || fail LOCKOUT 'passed G073 must render sourceEditLockout PASS'
  fi

  human_verdict_count="$(awk '/^verdict: / { count++ } END { print count + 0 }' "$human_block")"
  [[ "$human_verdict_count" -eq 1 ]] || fail PRESENTATION 'human view must contain exactly one verdict line'
  human_verdict="$(awk '/^verdict: / { print substr($0, 10); exit }' "$human_block")"
  assert_equal human.verdict "$human_verdict" "$audit_verdict"
  for human_value in "target:$target" "mode:$workflow_mode" "audit class:$audit_class" "ceiling:$status_ceiling"; do
    human_key="${human_value%%:*}"
    expected_human="${human_value#*:}"
    observed_human="$(awk -v prefix="$human_key: " 'index($0, prefix) == 1 { print substr($0, length(prefix) + 1); exit }' "$human_block")"
    assert_equal "human.$human_key" "$observed_human" "$expected_human"
  done

  if [[ "$target" == /* ]]; then
    target_path="$target"
  else
    target_path="$PWD/$target"
  fi
  state_file="$target_path/state.json"
  [[ -f "$state_file" ]] || fail PERSISTENCE "persisted target state is missing: $state_file"
  jq -e '
    .execution.audit.schemaVersion == "audit-run/v1"
    and (.execution.audit.runId | type == "string" and length > 0)
    and (.execution.audit.attempts | type == "array")
    and all(.execution.audit.attempts[];
      (.attemptId | type == "string" and length > 0)
      and (.resultState == "ACTIVE" or .resultState == "SUPERSEDED" or .resultState == "INCOMPLETE")
      and (.targetRevision | type == "string" and length > 0)
      and (.contractDigest | type == "string" and length > 0)
      and (.auditProfile | type == "string" and length > 0)
      and (.targetStatus | type == "string" and length > 0)
      and (.auditVerdict | type == "string" and length > 0)
      and (.outcome | type == "string" and length > 0)
      and (.evidenceRef | type == "string" and length > 0)
      and (.addressedFindings | type == "array")
      and (.unresolvedFindings | type == "array"))
  ' "$state_file" >/dev/null 2>&1 || fail PERSISTENCE 'execution.audit is malformed or not audit-run/v1'

  assert_equal persisted.runId "$(jq -r '.execution.audit.runId' "$state_file")" "$run_id"
  active_count="$(jq -r '[.execution.audit.attempts[] | select(.resultState == "ACTIVE")] | length' "$state_file")"
  [[ "$active_count" -le 1 ]] || fail PERSISTENCE 'multiple ACTIVE audit attempts are forbidden'
  current_attempt="$(jq -r 'if .execution.audit.currentAttemptId == null then "null" else .execution.audit.currentAttemptId end' "$state_file")"
  if [[ "$current_attempt" == "null" ]]; then
    [[ "$active_count" -eq 0 ]] || fail PERSISTENCE 'ACTIVE attempt exists while currentAttemptId is null'
  else
    [[ "$active_count" -eq 1 ]] || fail PERSISTENCE 'currentAttemptId requires exactly one ACTIVE attempt'
    [[ "$(jq -r --arg id "$current_attempt" '[.execution.audit.attempts[] | select(.attemptId == $id and .resultState == "ACTIVE")] | length' "$state_file")" -eq 1 ]] || fail PERSISTENCE 'currentAttemptId is dangling or does not point to ACTIVE'
  fi

  attempt_count="$(jq -r --arg id "$attempt_id" '[.execution.audit.attempts[] | select(.attemptId == $id)] | length' "$state_file")"
  [[ "$attempt_count" -eq 1 ]] || fail PERSISTENCE "attemptId '$attempt_id' must identify exactly one persisted attempt"
  attempt_state="$(jq -r --arg id "$attempt_id" '.execution.audit.attempts[] | select(.attemptId == $id) | .resultState' "$state_file")"
  assert_equal persisted.resultState "$attempt_state" "$result_state"
  if [[ "$result_state" == "ACTIVE" ]]; then
    [[ "$current_attempt" == "$attempt_id" ]] || fail PERSISTENCE 'ACTIVE result is not the current attempt'
  elif [[ "$result_state" == "INCOMPLETE" ]]; then
    [[ "$current_attempt" == "null" ]] || fail PERSISTENCE 'INCOMPLETE result must leave currentAttemptId null'
  else
    [[ "$current_attempt" != "$attempt_id" ]] || fail PERSISTENCE 'SUPERSEDED result cannot remain current'
  fi

  assert_equal persisted.targetRevision "$(jq -r --arg id "$attempt_id" '.execution.audit.attempts[] | select(.attemptId == $id) | .targetRevision' "$state_file")" "$target_revision"
  assert_equal persisted.contractDigest "$(jq -r --arg id "$attempt_id" '.execution.audit.attempts[] | select(.attemptId == $id) | .contractDigest' "$state_file")" "$contract_digest"
  assert_equal persisted.auditProfile "$(jq -r --arg id "$attempt_id" '.execution.audit.attempts[] | select(.attemptId == $id) | .auditProfile' "$state_file")" "$expected_profile"
  assert_equal persisted.targetStatus "$(jq -r --arg id "$attempt_id" '.execution.audit.attempts[] | select(.attemptId == $id) | .targetStatus' "$state_file")" "$status_ceiling"
  assert_equal persisted.auditVerdict "$(jq -r --arg id "$attempt_id" '.execution.audit.attempts[] | select(.attemptId == $id) | .auditVerdict' "$state_file")" "$audit_verdict"
  assert_equal persisted.outcome "$(jq -r --arg id "$attempt_id" '.execution.audit.attempts[] | select(.attemptId == $id) | .outcome' "$state_file")" "$outcome"
  assert_equal persisted.addressedFindings "$(jq -r --arg id "$attempt_id" '.execution.audit.attempts[] | select(.attemptId == $id) | "[" + (.addressedFindings | join(",")) + "]"' "$state_file")" "$addressed_findings"
  assert_equal persisted.unresolvedFindings "$(jq -r --arg id "$attempt_id" '.execution.audit.attempts[] | select(.attemptId == $id) | "[" + (.unresolvedFindings | join(",")) + "]"' "$state_file")" "$unresolved_findings"

  evidence_ref="$(jq -r --arg id "$attempt_id" '.execution.audit.attempts[] | select(.attemptId == $id) | .evidenceRef' "$state_file")"
  if [[ "$result_state" == "ACTIVE" ]]; then
    [[ "$evidence_ref" != "none" ]] || fail PERSISTENCE 'ACTIVE attempt requires an evidenceRef'
    list_contains "$evidence_refs" "$evidence_ref" || fail PERSISTENCE 'persisted evidenceRef is absent from AUDIT_RESULT_V1 evidenceRefs'
  fi

  attempt_total="$(jq -r '.execution.audit.attempts | length' "$state_file")"
  prior_attempt_count=$((attempt_total - 1))
  if [[ "$prior_attempt_count" -gt 0 && "$result_state" != "SUPERSEDED" ]]; then
    [[ "$supersedes_attempt_id" != "none" ]] || fail PERSISTENCE 'new attempt does not identify the superseded prior attempt'
    prior_state="$(jq -r --arg id "$supersedes_attempt_id" '[.execution.audit.attempts[] | select(.attemptId == $id and .resultState == "SUPERSEDED")] | length' "$state_file")"
    [[ "$prior_state" -eq 1 ]] || fail PERSISTENCE 'supersedesAttemptId does not identify one SUPERSEDED attempt'
  fi

  while IFS= read -r prior_finding || [[ -n "$prior_finding" ]]; do
    [[ -n "$prior_finding" ]] || continue
    if list_contains "$addressed_findings" "$prior_finding"; then
      continue
    fi
    list_contains "$unresolved_findings" "$prior_finding" || fail FINDINGS "prior finding '$prior_finding' disappeared"
  done < <(jq -r --arg id "$attempt_id" '.execution.audit.attempts[] | select(.attemptId != $id) | (.addressedFindings[]?, .unresolvedFindings[]?)' "$state_file")

  if [[ "$contract_digest" != "UNRESOLVED" ]]; then
    [[ "$contract_digest" =~ ^sha256:[0-9a-f]{64}$ ]] || fail PROVENANCE 'contractDigest must be sha256:HEX'
  else
    [[ "$audit_verdict" == "BLOCKED" ]] || fail PROVENANCE 'UNRESOLVED contractDigest requires BLOCKED'
  fi
  if [[ "$target_revision" != "UNRESOLVED" ]]; then
    [[ "$target_revision" =~ ^sha256:[0-9a-f]{64}$ ]] || fail PROVENANCE 'targetRevision must be sha256:HEX'
  else
    [[ "$audit_verdict" == "BLOCKED" ]] || fail PROVENANCE 'UNRESOLVED targetRevision requires BLOCKED'
  fi
  [[ "$contract_ref" != "none" || "$audit_verdict" == "BLOCKED" ]] || fail PROVENANCE 'non-blocked result requires contractRef'

  rm -f "$audit_block" "$transition_block" "$human_block"
  trap - EXIT INT TERM
  printf 'audit-result-contract-lint: PASS result %s (%s/%s)\n' "$result_file" "$audit_class" "$audit_verdict"
}

if [[ $# -ne 2 ]]; then
  usage >&2
  exit 64
fi

case "$1" in
  --result)
    validate_result_contract "$2"
    ;;
  --agent-contract)
    validate_agent_contract "$2"
    ;;
  -h|--help)
    usage
    ;;
  *)
    usage >&2
    exit 64
    ;;
esac