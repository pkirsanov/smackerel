#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
LINT="$SCRIPT_DIR/audit-result-contract-lint.sh"
GUARD_LIB="$SCRIPT_DIR/guard-lib.sh"
AGENT_CONTRACT="$REPO_ROOT/agents/bubbles.audit.agent.md"
VALIDATION_PROFILES="$REPO_ROOT/agents/bubbles_shared/validation-profiles.md"

for required_file in "$LINT" "$GUARD_LIB" "$AGENT_CONTRACT" "$VALIDATION_PROFILES"; do
  if [[ ! -f "$required_file" ]]; then
    printf 'audit-result-contract-lint-selftest: FAIL: required file missing: %s\n' "$required_file" >&2
    exit 1
  fi
done

if ! command -v jq >/dev/null 2>&1; then
  printf 'audit-result-contract-lint-selftest: SKIP (jq not installed)\n'
  exit 0
fi

# shellcheck source=/dev/null
source "$GUARD_LIB"

LC_ALL=C
export LC_ALL

WORKSPACE="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-audit-result-selftest.XXXXXXXX")"
cleanup() {
  rm -rf "$WORKSPACE"
}
trap cleanup EXIT INT TERM

PASS_COUNT=0
FAIL_COUNT=0
DIGEST="sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
REVISION="sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

pass() {
  PASS_COUNT=$((PASS_COUNT + 1))
  printf 'PASS: %s\n' "$1"
}

fail() {
  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf 'FAIL: %s\n' "$1" >&2
}

run_pass() {
  local label="$1"
  local result_file="$2"
  local output_file="$WORKSPACE/pass-$PASS_COUNT.log"
  if bash "$LINT" --result "$result_file" > "$output_file" 2>&1; then
    pass "$label"
  else
    fail "$label"
    cat "$output_file" >&2
  fi
}

run_fail() {
  local label="$1"
  local result_file="$2"
  local expected="$3"
  local output_file="$WORKSPACE/fail-$PASS_COUNT-$FAIL_COUNT.log"
  local exit_code
  set +e
  bash "$LINT" --result "$result_file" > "$output_file" 2>&1
  exit_code=$?
  set -e
  if [[ "$exit_code" -ne 0 ]] && grep -Fq -- "$expected" "$output_file"; then
    pass "$label"
  else
    fail "$label (exit=$exit_code expected=$expected)"
    cat "$output_file" >&2
  fi
}

write_state() {
  local target="$1"
  local run_id="$2"
  local current_attempt="$3"
  local attempts_json="$4"
  mkdir -p "$target"
  cat > "$target/state.json" <<EOF
{
  "execution": {
    "audit": {
      "schemaVersion": "audit-run/v1",
      "runId": "$run_id",
      "currentAttemptId": $current_attempt,
      "attempts": $attempts_json
    }
  }
}
EOF
}

attempt_json() {
  local attempt_id="$1"
  local result_state="$2"
  local revision="$3"
  local digest="$4"
  local profile="$5"
  local target_status="$6"
  local verdict="$7"
  local outcome="$8"
  local evidence_ref="$9"
  local addressed="${10}"
  local unresolved="${11}"
  printf '{"attemptId":"%s","resultState":"%s","targetRevision":"%s","contractDigest":"%s","auditProfile":"%s","targetStatus":"%s","auditVerdict":"%s","outcome":"%s","evidenceRef":"%s","addressedFindings":%s,"unresolvedFindings":%s}' \
    "$attempt_id" "$result_state" "$revision" "$digest" "$profile" "$target_status" "$verdict" "$outcome" "$evidence_ref" "$addressed" "$unresolved"
}

write_transition_block() {
  local file="$1"
  local workflow_mode="$2"
  local profile="$3"
  local target_status="$4"
  local digest="$5"
  local revision="$6"
  local classes="$7"
  local not_applicable="$8"
  local passed_gates="$9"
  local failed_gates="${10}"
  local failed_checks="${11}"
  local blocking_code="${12}"
  local failure_count="${13}"
  local exit_status="${14}"
  local verdict="${15}"
  cat > "$file" <<EOF
BEGIN TRANSITION_GUARD_RESULT_V1
schemaVersion: transition-guard-result/v1
workflowMode: $workflow_mode
auditProfile: $profile
targetStatus: $target_status
contractDigest: $digest
targetRevision: $revision
applicableCheckClasses: $classes
notApplicableChecks: $not_applicable
passedGateIds: $passed_gates
failedGateIds: $failed_gates
failedChecks: $failed_checks
blockingCode: $blocking_code
failureCount: $failure_count
exitStatus: $exit_status
verdict: $verdict
END TRANSITION_GUARD_RESULT_V1
EOF
}

append_human_view() {
  local file="$1"
  local target="$2"
  local mode="$3"
  local audit_class="$4"
  local ceiling="$5"
  local verdict="$6"
  cat >> "$file" <<EOF
AUDIT RESULT
target: $target
mode: $mode
audit class: $audit_class
ceiling: $ceiling
verdict: $verdict

EVALUATION
EOF
}

append_audit_block() {
  local file="$1"
  local run_id="$2"
  local attempt_id="$3"
  local target="$4"
  local revision="$5"
  local workflow_mode="$6"
  local mode_class="$7"
  local audit_class="$8"
  local ceiling="$9"
  local requested_status="${10}"
  local verdict="${11}"
  local outcome="${12}"
  local result_state="${13}"
  local certified_status="${14}"
  local planning_evaluation="${15}"
  local delivery_evaluation="${16}"
  local source_lockout="${17}"
  local classes="${18}"
  local not_applicable="${19}"
  local passed_gates="${20}"
  local failed_gates="${21}"
  local failed_checks="${22}"
  local blocking_code="${23}"
  local unresolved_fields="${24}"
  local contradictions="${25}"
  local contract_ref="${26}"
  local digest="${27}"
  local evidence_refs="${28}"
  local addressed="${29}"
  local unresolved="${30}"
  local next_owner="${31}"
  local supersedes="${32}"
  local resume_phase="${33}"
  cat >> "$file" <<EOF
BEGIN AUDIT_RESULT_V1
schemaVersion: audit-result/v1
runId: $run_id
attemptId: $attempt_id
target: $target
targetRevision: $revision
workflowMode: $workflow_mode
modeClass: $mode_class
auditClass: $audit_class
statusCeiling: $ceiling
requestedStatus: $requested_status
auditVerdict: $verdict
outcome: $outcome
resultState: $result_state
certifiedStatus: $certified_status
planningEvaluation: $planning_evaluation
deliveryEvaluation: $delivery_evaluation
sourceEditLockout: $source_lockout
applicableCheckClasses: $classes
notApplicableChecks: $not_applicable
passedGateIds: $passed_gates
failedGateIds: $failed_gates
failedChecks: $failed_checks
blockingCode: $blocking_code
unresolvedFields: $unresolved_fields
contradictions: $contradictions
contractRef: $contract_ref
contractDigest: $digest
evidenceRefs: $evidence_refs
addressedFindings: $addressed
unresolvedFindings: $unresolved
nextRequiredOwner: $next_owner
supersedesAttemptId: $supersedes
resumeFromPhase: $resume_phase
END AUDIT_RESULT_V1
EOF
}

write_planning_clean() {
  local case_dir="$1"
  local target="$case_dir/target"
  local result="$case_dir/result.txt"
  local attempt
  mkdir -p "$case_dir"
  attempt="$(attempt_json attempt-clean ACTIVE "$REVISION" "$DIGEST" planning-maturity-v1 specs_hardened PLANNING_AUDIT_CLEAN completed_diagnostic report.md#audit-attempt-clean '[]' '[]')"
  write_state "$target" run-clean '"attempt-clean"' "[$attempt]"
  write_transition_block "$result" product-to-planning planning-maturity-v1 specs_hardened "$DIGEST" "$REVISION" '[universal,planning-maturity]' '[Check-4-completion,Check-5-all-done,Check-8-file-existence,Check-11-execution-evidence]' '[G040,G068,G073,G087,G091]' '[]' '[]' none 0 0 PASS
  append_human_view "$result" "$target" product-to-planning planning-maturity specs_hardened PLANNING_AUDIT_CLEAN
  cat >> "$result" <<'EOF'
planning: planning ceiling certified
delivery: delivery not evaluated
not applicable: Check-4-completion, Check-5-all-done, Check-8-file-existence, Check-11-execution-evidence
EOF
  append_audit_block "$result" run-clean attempt-clean "$target" "$REVISION" product-to-planning none planning-maturity specs_hardened specs_hardened PLANNING_AUDIT_CLEAN completed_diagnostic ACTIVE specs_hardened CERTIFIED NOT_EVALUATED PASS '[universal,planning-maturity]' '[Check-4-completion,Check-5-all-done,Check-8-file-existence,Check-11-execution-evidence]' '[G040,G068,G073,G087,G091]' '[]' '[]' none '[]' '[]' bubbles/workflows/modes.yaml#product-to-planning "$DIGEST" '[report.md#audit-attempt-clean]' '[]' '[]' none none none
}

write_planning_rework() {
  local case_dir="$1"
  local target="$case_dir/target"
  local result="$case_dir/result.txt"
  local attempt
  mkdir -p "$case_dir"
  attempt="$(attempt_json attempt-rework ACTIVE "$REVISION" "$DIGEST" planning-maturity-v1 specs_hardened PLANNING_REWORK_REQUIRED route_required report.md#audit-attempt-rework '[]' '["F009-G068"]')"
  write_state "$target" run-rework '"attempt-rework"' "[$attempt]"
  write_transition_block "$result" product-to-planning planning-maturity-v1 specs_hardened "$DIGEST" "$REVISION" '[universal,planning-maturity]' '[Check-4-completion,Check-5-all-done,Check-8-file-existence,Check-11-execution-evidence]' '[G040,G073,G087,G091]' '[G068]' '[]' TRANSITION_GUARD_FAILED 1 1 FAIL
  append_human_view "$result" "$target" product-to-planning planning-maturity specs_hardened PLANNING_REWORK_REQUIRED
  cat >> "$result" <<'EOF'
planning: planning rework required
delivery: delivery not evaluated
not applicable: Check-4-completion, Check-5-all-done, Check-8-file-existence, Check-11-execution-evidence
next owner: bubbles.plan
EOF
  append_audit_block "$result" run-rework attempt-rework "$target" "$REVISION" product-to-planning none planning-maturity specs_hardened specs_hardened PLANNING_REWORK_REQUIRED route_required ACTIVE none REWORK_REQUIRED NOT_EVALUATED PASS '[universal,planning-maturity]' '[Check-4-completion,Check-5-all-done,Check-8-file-existence,Check-11-execution-evidence]' '[G040,G073,G087,G091]' '[G068]' '[]' PLANNING_GATE_FAILED '[]' '[]' bubbles/workflows/modes.yaml#product-to-planning "$DIGEST" '[report.md#audit-attempt-rework]' '[]' '[F009-G068]' bubbles.plan none none
}

write_delivery_refusal() {
  local case_dir="$1"
  local target="$case_dir/target"
  local result="$case_dir/result.txt"
  local attempt
  mkdir -p "$case_dir"
  attempt="$(attempt_json attempt-delivery ACTIVE "$REVISION" "$DIGEST" delivery-completion-v1 "done" DO_NOT_SHIP route_required report.md#audit-attempt-delivery '[]' '["F009-DELIVERY"]')"
  write_state "$target" run-delivery '"attempt-delivery"' "[$attempt]"
  write_transition_block "$result" full-delivery delivery-completion-v1 "done" "$DIGEST" "$REVISION" '[universal,delivery-completion]' '[]' '[G073]' '[]' '[Check-4-completion,Check-5-all-done,Check-8-file-existence,Check-11-execution-evidence]' TRANSITION_GUARD_FAILED 4 1 FAIL
  append_human_view "$result" "$target" full-delivery delivery-completion "done" DO_NOT_SHIP
  cat >> "$result" <<'EOF'
planning: not separately certified
delivery: delivery refused
next owner: bubbles.implement
EOF
  append_audit_block "$result" run-delivery attempt-delivery "$target" "$REVISION" full-delivery feature-delivery delivery-completion "done" "done" DO_NOT_SHIP route_required ACTIVE none NOT_EVALUATED REFUSED PASS '[universal,delivery-completion]' '[]' '[G073]' '[]' '[Check-4-completion,Check-5-all-done,Check-8-file-existence,Check-11-execution-evidence]' DELIVERY_COMPLETION_FAILED '[]' '[]' bubbles/workflows/modes.yaml#full-delivery "$DIGEST" '[report.md#audit-attempt-delivery]' '[]' '[F009-DELIVERY]' bubbles.implement none none
}

write_metadata_blocked() {
  local case_dir="$1"
  local target="$case_dir/target"
  local result="$case_dir/result.txt"
  local attempt
  mkdir -p "$case_dir"
  attempt="$(attempt_json attempt-metadata ACTIVE UNRESOLVED UNRESOLVED UNRESOLVED UNRESOLVED BLOCKED blocked report.md#audit-attempt-metadata '[]' '["F009-METADATA"]')"
  write_state "$target" run-metadata '"attempt-metadata"' "[$attempt]"
  write_transition_block "$result" UNRESOLVED UNRESOLVED UNRESOLVED UNRESOLVED UNRESOLVED '[]' '[]' '[]' '[]' '[contract-resolution]' E009-MODE-UNKNOWN 1 2 BLOCKED
  append_human_view "$result" "$target" UNRESOLVED UNRESOLVED UNRESOLVED BLOCKED
  cat >> "$result" <<'EOF'
planning: not evaluated
delivery: not evaluated
next owner: bubbles.super
EOF
  append_audit_block "$result" run-metadata attempt-metadata "$target" UNRESOLVED UNRESOLVED UNRESOLVED UNRESOLVED UNRESOLVED none BLOCKED blocked ACTIVE none NOT_EVALUATED NOT_EVALUATED NOT_EVALUATED '[]' '[]' '[]' '[]' '[contract-resolution]' AUDIT_CONTRACT_UNRESOLVED '[workflowMode,statusCeiling,auditClass]' '[]' bubbles/workflows/modes.yaml UNRESOLVED '[report.md#audit-attempt-metadata]' '[]' '[F009-METADATA]' bubbles.super none none
}

write_source_lockout() {
  local case_dir="$1"
  local target="$case_dir/target"
  local result="$case_dir/result.txt"
  local attempt
  mkdir -p "$case_dir"
  attempt="$(attempt_json attempt-lockout ACTIVE "$REVISION" "$DIGEST" planning-maturity-v1 specs_hardened BLOCKED blocked report.md#audit-attempt-lockout '[]' '["F009-G073"]')"
  write_state "$target" run-lockout '"attempt-lockout"' "[$attempt]"
  write_transition_block "$result" product-to-planning planning-maturity-v1 specs_hardened "$DIGEST" "$REVISION" '[universal,planning-maturity]' '[Check-4-completion,Check-5-all-done,Check-8-file-existence,Check-11-execution-evidence]' '[]' '[G073]' '[source-edit-lockout]' TRANSITION_GUARD_FAILED 1 1 FAIL
  append_human_view "$result" "$target" product-to-planning planning-maturity specs_hardened BLOCKED
  cat >> "$result" <<'EOF'
planning: not evaluated
delivery: delivery not evaluated
not applicable: Check-4-completion, Check-5-all-done, Check-8-file-existence, Check-11-execution-evidence
next owner: bubbles.super
EOF
  append_audit_block "$result" run-lockout attempt-lockout "$target" "$REVISION" product-to-planning none planning-maturity specs_hardened specs_hardened BLOCKED blocked ACTIVE none NOT_EVALUATED NOT_EVALUATED FAIL '[universal,planning-maturity]' '[Check-4-completion,Check-5-all-done,Check-8-file-existence,Check-11-execution-evidence]' '[]' '[G073]' '[source-edit-lockout]' SOURCE_EDIT_LOCKOUT '[]' '[]' bubbles/workflows/modes.yaml#product-to-planning "$DIGEST" '[report.md#audit-attempt-lockout]' '[]' '[F009-G073]' bubbles.super none none
}

write_interrupted() {
  local case_dir="$1"
  local target="$case_dir/target"
  local result="$case_dir/result.txt"
  local prior
  local current
  mkdir -p "$case_dir"
  prior="$(attempt_json attempt-prior SUPERSEDED "$REVISION" "$DIGEST" planning-maturity-v1 specs_hardened PLANNING_REWORK_REQUIRED route_required report.md#audit-attempt-prior '[]' '["F009-OPEN"]')"
  current="$(attempt_json attempt-interrupted INCOMPLETE "$REVISION" "$DIGEST" planning-maturity-v1 specs_hardened INTERRUPTED blocked none '[]' '["F009-OPEN"]')"
  write_state "$target" run-interrupted null "[$prior,$current]"
  write_transition_block "$result" product-to-planning planning-maturity-v1 specs_hardened "$DIGEST" "$REVISION" '[universal,planning-maturity]' '[Check-4-completion,Check-5-all-done,Check-8-file-existence,Check-11-execution-evidence]' '[G040,G068,G073,G087,G091]' '[]' '[]' none 0 0 PASS
  append_human_view "$result" "$target" product-to-planning planning-maturity specs_hardened INTERRUPTED
  cat >> "$result" <<'EOF'
planning: not evaluated
delivery: delivery not evaluated
not applicable: Check-4-completion, Check-5-all-done, Check-8-file-existence, Check-11-execution-evidence
next owner: bubbles.audit
EOF
  append_audit_block "$result" run-interrupted attempt-interrupted "$target" "$REVISION" product-to-planning none planning-maturity specs_hardened specs_hardened INTERRUPTED blocked INCOMPLETE none NOT_EVALUATED NOT_EVALUATED PASS '[universal,planning-maturity]' '[Check-4-completion,Check-5-all-done,Check-8-file-existence,Check-11-execution-evidence]' '[G040,G068,G073,G087,G091]' '[]' '[]' AUDIT_INTERRUPTED '[]' '[]' bubbles/workflows/modes.yaml#product-to-planning "$DIGEST" '[guard-attempt-interrupted]' '[]' '[F009-OPEN]' bubbles.audit attempt-prior 1
}

write_rework_closed() {
  local case_dir="$1"
  local target="$case_dir/target"
  local result="$case_dir/result.txt"
  local prior
  local current
  mkdir -p "$case_dir"
  prior="$(attempt_json attempt-old SUPERSEDED "$REVISION" "$DIGEST" planning-maturity-v1 specs_hardened PLANNING_REWORK_REQUIRED route_required report.md#audit-attempt-old '[]' '["F009-CLOSED"]')"
  current="$(attempt_json attempt-new ACTIVE "$REVISION" "$DIGEST" planning-maturity-v1 specs_hardened PLANNING_AUDIT_CLEAN completed_diagnostic report.md#audit-attempt-new '["F009-CLOSED"]' '[]')"
  write_state "$target" run-rework-closed '"attempt-new"' "[$prior,$current]"
  write_transition_block "$result" product-to-planning planning-maturity-v1 specs_hardened "$DIGEST" "$REVISION" '[universal,planning-maturity]' '[Check-4-completion,Check-5-all-done,Check-8-file-existence,Check-11-execution-evidence]' '[G040,G068,G073,G087,G091]' '[]' '[]' none 0 0 PASS
  append_human_view "$result" "$target" product-to-planning planning-maturity specs_hardened PLANNING_AUDIT_CLEAN
  cat >> "$result" <<'EOF'
planning: planning ceiling certified
delivery: delivery not evaluated
not applicable: Check-4-completion, Check-5-all-done, Check-8-file-existence, Check-11-execution-evidence
EOF
  append_audit_block "$result" run-rework-closed attempt-new "$target" "$REVISION" product-to-planning none planning-maturity specs_hardened specs_hardened PLANNING_AUDIT_CLEAN completed_diagnostic ACTIVE specs_hardened CERTIFIED NOT_EVALUATED PASS '[universal,planning-maturity]' '[Check-4-completion,Check-5-all-done,Check-8-file-existence,Check-11-execution-evidence]' '[G040,G068,G073,G087,G091]' '[]' '[]' none '[]' '[]' bubbles/workflows/modes.yaml#product-to-planning "$DIGEST" '[report.md#audit-attempt-new]' '[F009-CLOSED]' '[]' none attempt-old none
}

mutate_result() {
  local source="$1"
  local destination="$2"
  local expression="$3"
  cp "$source" "$destination"
  bubbles_sed_inplace "$expression" "$destination"
}

printf 'Running BUG-009 S04 audit result contract selftest...\n'

PLANNING_CLEAN="$WORKSPACE/planning-clean"
PLANNING_REWORK="$WORKSPACE/planning-rework"
DELIVERY_REFUSAL="$WORKSPACE/delivery-refusal"
METADATA_BLOCKED="$WORKSPACE/metadata-blocked"
SOURCE_LOCKOUT="$WORKSPACE/source-lockout"
INTERRUPTED="$WORKSPACE/interrupted"
REWORK_CLOSED="$WORKSPACE/rework-closed"

write_planning_clean "$PLANNING_CLEAN"
write_planning_rework "$PLANNING_REWORK"
write_delivery_refusal "$DELIVERY_REFUSAL"
write_metadata_blocked "$METADATA_BLOCKED"
write_source_lockout "$SOURCE_LOCKOUT"
write_interrupted "$INTERRUPTED"
write_rework_closed "$REWORK_CLOSED"

run_pass 'planning clean view is exact and profile-bound' "$PLANNING_CLEAN/result.txt"
run_pass 'planning rework view names a concrete owner' "$PLANNING_REWORK/result.txt"
run_pass 'delivery refusal preserves DO_NOT_SHIP semantics' "$DELIVERY_REFUSAL/result.txt"
run_pass 'metadata uncertainty is BLOCKED without fallback semantics' "$METADATA_BLOCKED/result.txt"
run_pass 'source-edit lockout is BLOCKED on G073' "$SOURCE_LOCKOUT/result.txt"
run_pass 'interruption leaves no current pointer or active verdict' "$INTERRUPTED/result.txt"
run_pass 'rework supersedes prior result and preserves the finding one-to-one' "$REWORK_CLOSED/result.txt"

DUPLICATE="$WORKSPACE/duplicate.txt"
cp "$PLANNING_CLEAN/result.txt" "$DUPLICATE"
awk '/BEGIN AUDIT_RESULT_V1/,/END AUDIT_RESULT_V1/' "$PLANNING_CLEAN/result.txt" >> "$DUPLICATE"
run_fail 'duplicate AUDIT_RESULT_V1 block is rejected' "$DUPLICATE" 'exactly one AUDIT_RESULT_V1 begin marker'

MISSING="$WORKSPACE/missing.txt"
awk '$0 !~ /^resumeFromPhase: /' "$PLANNING_CLEAN/result.txt" > "$MISSING"
run_fail 'missing frozen field is rejected' "$MISSING" 'audit block has'

REORDERED="$WORKSPACE/reordered.txt"
awk '
  /^runId: / { run=$0; next }
  /^attemptId: / { print; print run; next }
  { print }
' "$PLANNING_CLEAN/result.txt" > "$REORDERED"
run_fail 'reordered frozen fields are rejected' "$REORDERED" "must be 'runId'"

MALFORMED="$WORKSPACE/malformed.txt"
mutate_result "$PLANNING_CLEAN/result.txt" "$MALFORMED" 's/^passedGateIds: \[G040,G068,G073,G087,G091\]$/passedGateIds: G040,G068/'
run_fail 'malformed collection is rejected' "$MALFORMED" 'collection syntax'

SHIPMENT="$WORKSPACE/shipment.txt"
awk '/^BEGIN AUDIT_RESULT_V1$/ { print "workflow action: approved for merge and shipped" } { print }' "$PLANNING_CLEAN/result.txt" > "$SHIPMENT"
run_fail 'planning shipment language is rejected' "$SHIPMENT" 'shipment or positive delivery language'

NA_PASS="$WORKSPACE/not-applicable-pass.txt"
awk '/^BEGIN AUDIT_RESULT_V1$/ { print "Check-4-completion: PASS" } { print }' "$PLANNING_CLEAN/result.txt" > "$NA_PASS"
run_fail 'planning PASS claim for non-applicable delivery check is rejected' "$NA_PASS" 'reported as PASS/CERTIFIED'

STALE_DIGEST="$WORKSPACE/stale-digest.txt"
cp "$PLANNING_CLEAN/result.txt" "$STALE_DIGEST"
awk '
  /^BEGIN AUDIT_RESULT_V1$/ { audit=1 }
  audit && /^contractDigest: / { print "contractDigest: sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"; next }
  { print }
' "$PLANNING_CLEAN/result.txt" > "$STALE_DIGEST"
run_fail 'stale contract digest is rejected against guard provenance' "$STALE_DIGEST" 'guard.contractDigest mismatch'

STALE_REVISION="$WORKSPACE/stale-revision.txt"
awk '
  /^BEGIN AUDIT_RESULT_V1$/ { audit=1 }
  audit && /^targetRevision: / { print "targetRevision: sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"; next }
  { print }
' "$PLANNING_CLEAN/result.txt" > "$STALE_REVISION"
run_fail 'stale target revision is rejected against guard provenance' "$STALE_REVISION" 'guard.targetRevision mismatch'

DELIVERY_DRIFT="$WORKSPACE/delivery-drift.txt"
mutate_result "$DELIVERY_REFUSAL/result.txt" "$DELIVERY_DRIFT" 's/^deliveryEvaluation: REFUSED$/deliveryEvaluation: NOT_EVALUATED/'
run_fail 'delivery verdict drift is rejected' "$DELIVERY_DRIFT" 'delivery refusal field combination is inconsistent'

COLOR="$WORKSPACE/color.txt"
awk 'NR == 18 { printf "\033[31m" } { print }' "$PLANNING_CLEAN/result.txt" > "$COLOR"
run_fail 'ANSI/color output is rejected' "$COLOR" 'non-ASCII, control, color'

MULTI_ACTIVE="$WORKSPACE/multiple-active"
cp -R "$REWORK_CLOSED" "$MULTI_ACTIVE"
jq '.execution.audit.attempts[0].resultState = "ACTIVE"' "$MULTI_ACTIVE/target/state.json" > "$MULTI_ACTIVE/state.tmp"
mv "$MULTI_ACTIVE/state.tmp" "$MULTI_ACTIVE/target/state.json"
bubbles_sed_inplace "s|$REWORK_CLOSED/target|$MULTI_ACTIVE/target|g" "$MULTI_ACTIVE/result.txt"
run_fail 'multiple ACTIVE attempts are rejected' "$MULTI_ACTIVE/result.txt" 'multiple ACTIVE audit attempts'

DANGLING="$WORKSPACE/dangling"
cp -R "$PLANNING_CLEAN" "$DANGLING"
jq '.execution.audit.currentAttemptId = "missing-attempt"' "$DANGLING/target/state.json" > "$DANGLING/state.tmp"
mv "$DANGLING/state.tmp" "$DANGLING/target/state.json"
bubbles_sed_inplace "s|$PLANNING_CLEAN/target|$DANGLING/target|g" "$DANGLING/result.txt"
run_fail 'dangling currentAttemptId is rejected' "$DANGLING/result.txt" 'dangling or does not point to ACTIVE'

DISAPPEARING="$WORKSPACE/disappearing"
cp -R "$REWORK_CLOSED" "$DISAPPEARING"
jq '.execution.audit.attempts[1].addressedFindings = []' "$DISAPPEARING/target/state.json" > "$DISAPPEARING/state.tmp"
mv "$DISAPPEARING/state.tmp" "$DISAPPEARING/target/state.json"
bubbles_sed_inplace "s|$REWORK_CLOSED/target|$DISAPPEARING/target|g; s/^addressedFindings: \[F009-CLOSED\]$/addressedFindings: []/" "$DISAPPEARING/result.txt"
run_fail 'disappearing prior finding is rejected' "$DISAPPEARING/result.txt" "prior finding 'F009-CLOSED' disappeared"

if bash "$LINT" --agent-contract "$AGENT_CONTRACT" > "$WORKSPACE/agent-contract.log" 2>&1; then
  pass 'canonical audit agent passes structural contract lint'
else
  fail 'canonical audit agent fails structural contract lint'
  cat "$WORKSPACE/agent-contract.log" >&2
fi

if awk -F '|' '
  $2 ~ /^[[:space:]]*A1[[:space:]]*$/ &&
  $3 ~ /Profile-scoped state transition guard passes/ &&
  $4 ~ /registry-resolved profile/ { found=1 }
  END { exit(found ? 0 : 1) }
' "$VALIDATION_PROFILES"; then
  pass 'Audit A1 wording is profile-scoped and registry-resolved'
else
  fail 'Audit A1 wording is not profile-scoped and registry-resolved'
fi

if [[ "$FAIL_COUNT" -ne 0 ]]; then
  printf 'audit-result-contract-lint-selftest: %s passed, %s failed\n' "$PASS_COUNT" "$FAIL_COUNT" >&2
  exit 1
fi

printf 'audit-result-contract-lint-selftest: %s passed, 0 failed\n' "$PASS_COUNT"