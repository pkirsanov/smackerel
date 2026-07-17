#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FRAMEWORK_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
WORKFLOWS_FILE="$FRAMEWORK_DIR/workflows.yaml"
MODES_FILE="$FRAMEWORK_DIR/workflows/modes.yaml"
MODE_RESOLVER="$SCRIPT_DIR/mode-resolver.sh"
GUARD_LIB="$SCRIPT_DIR/guard-lib.sh"
TRUST_METADATA="$SCRIPT_DIR/trust-metadata.sh"

fail() {
  local exit_code="$1"
  local error_code="$2"
  local detail="$3"
  printf '%s: %s\n' "$error_code" "$detail" >&2
  exit "$exit_code"
}

usage_error() {
  fail 64 E009-USAGE "$1"
}

if [[ -n "${AUDIT_PROFILE+x}" \
  || -n "${TRANSITION_AUDIT_PROFILE+x}" \
  || -n "${BUBBLES_AUDIT_PROFILE+x}" \
  || -n "${BUBBLES_TRANSITION_AUDIT_PROFILE+x}" \
  || -n "${BUBBLES_WORKFLOWS_FILE+x}" \
  || -n "${BUBBLES_MODES_FILE+x}" \
  || -n "${BUBBLES_WORKFLOW_ALIASES_FILE+x}" \
  || -n "${BUBBLES_MODE_GRANDFATHER+x}" ]]; then
  usage_error "workflow registry and audit profile environment overrides are not accepted"
fi

if (( $# == 0 )); then
  usage_error "FEATURE_DIR is required"
fi

feature_input="$1"
shift
if [[ -z "$feature_input" || "$feature_input" == --* ]]; then
  usage_error "FEATURE_DIR must be the first argument"
fi

expect_mode=""
expect_target=""
expect_contract_digest=""
while (( $# > 0 )); do
  case "$1" in
    --expect-mode)
      (( $# >= 2 )) || usage_error "--expect-mode requires a value"
      expect_mode="$2"
      shift 2
      ;;
    --expect-target)
      (( $# >= 2 )) || usage_error "--expect-target requires a value"
      expect_target="$2"
      shift 2
      ;;
    --expect-contract-digest)
      (( $# >= 2 )) || usage_error "--expect-contract-digest requires a value"
      expect_contract_digest="$2"
      shift 2
      ;;
    *)
      usage_error "unknown or policy-selecting argument: $1"
      ;;
  esac
done

for required_file in \
  "$WORKFLOWS_FILE" \
  "$MODES_FILE" \
  "$MODE_RESOLVER" \
  "$GUARD_LIB" \
  "$TRUST_METADATA"; do
  if [[ ! -f "$required_file" ]]; then
    fail 66 E009-REGISTRY-MISSING "required framework registry surface is unavailable"
  fi
done

for required_command in jq yq; do
  if ! command -v "$required_command" >/dev/null 2>&1; then
    fail 66 E009-REGISTRY-MISSING "required registry parser is unavailable"
  fi
done
if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
  fail 66 E009-REGISTRY-MISSING "required SHA-256 implementation is unavailable"
fi

# shellcheck source=/dev/null
source "$GUARD_LIB"
# shellcheck source=/dev/null
source "$TRUST_METADATA"

if [[ ! -d "$feature_input" ]]; then
  fail 65 E009-STATE-MALFORMED "feature directory does not exist"
fi
feature_dir="$(cd "$feature_input" && pwd -P)"
feature_display="${feature_input%/}"
if [[ "$feature_display" == ./* ]]; then
  feature_display="${feature_display#./}"
fi
case "$feature_dir" in
  */specs/*)
    feature_display="specs/${feature_dir#*/specs/}"
    ;;
esac
state_file="$feature_dir/state.json"
if [[ ! -f "$state_file" ]] || ! jq empty "$state_file" >/dev/null 2>&1; then
  fail 65 E009-STATE-MALFORMED "state.json is missing or is not valid JSON"
fi

workflow_mode="$(jq -r 'if (.workflowMode | type) == "string" then .workflowMode else "" end' "$state_file")"
current_status="$(jq -r 'if (.status | type) == "string" then .status else "" end' "$state_file")"
if [[ -z "$workflow_mode" || ! "$workflow_mode" =~ ^[a-z][a-z0-9-]*$ || -z "$current_status" ]]; then
  fail 65 E009-STATE-MALFORMED "state.json requires string workflowMode and status fields"
fi

policy_snapshot_type="$(jq -r 'if has("policySnapshot") then (.policySnapshot | type) else "missing" end' "$state_file")"
if [[ "$policy_snapshot_type" != "missing" && "$policy_snapshot_type" != "object" ]]; then
  fail 65 E009-STATE-MALFORMED "policySnapshot must be an object when present"
fi
policy_mode_type="$(jq -r 'if ((.policySnapshot // {}) | has("workflowMode")) then (.policySnapshot.workflowMode | type) else "missing" end' "$state_file")"
if [[ "$policy_mode_type" != "missing" && "$policy_mode_type" != "string" ]]; then
  fail 65 E009-STATE-MALFORMED "policySnapshot.workflowMode must be a string when present"
fi
policy_mode="$(jq -r '.policySnapshot.workflowMode // ""' "$state_file")"
if [[ -n "$policy_mode" && "$policy_mode" != "$workflow_mode" ]]; then
  fail 68 E009-STATE-MODE-MISMATCH "state and policy snapshot workflow modes disagree"
fi

certification_type="$(jq -r 'if has("certification") then (.certification | type) else "missing" end' "$state_file")"
if [[ "$certification_type" != "missing" && "$certification_type" != "object" ]]; then
  fail 65 E009-STATE-MALFORMED "certification must be an object when present"
fi
certification_status_type="$(jq -r 'if ((.certification // {}) | has("status")) then (.certification.status | type) else "missing" end' "$state_file")"
if [[ "$certification_status_type" != "missing" && "$certification_status_type" != "string" ]]; then
  fail 65 E009-STATE-MALFORMED "certification.status must be a string when present"
fi
certification_status="$(jq -r '.certification.status // ""' "$state_file")"
if [[ -n "$certification_status" && "$certification_status" != "$current_status" ]]; then
  fail 69 E009-TARGET-MISMATCH "top-level and certification status mirrors disagree"
fi

if ! MODE_NAME="$workflow_mode" yq -e '.modes[strenv(MODE_NAME)] | type == "!!map"' "$MODES_FILE" >/dev/null 2>&1; then
  fail 67 E009-MODE-UNKNOWN "persisted workflow mode is absent from the canonical registry"
fi

tmp_base="${TMPDIR:-/tmp}"
tmp_dir="$(mktemp -d "$tmp_base/bubbles-transition-contract.XXXXXX")"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

resolved_mode_file="$tmp_dir/resolved-mode.yaml"
resolver_error_file="$tmp_dir/mode-resolver.err"
if ! bash "$MODE_RESOLVER" --grandfather "$workflow_mode" > "$resolved_mode_file" 2> "$resolver_error_file"; then
  fail 67 E009-MODE-UNKNOWN "persisted workflow mode cannot be resolved canonically"
fi
if [[ "$(yq -r 'type' "$resolved_mode_file" 2>/dev/null)" != "!!map" ]]; then
  fail 68 E009-STATE-MODE-MISMATCH "canonical mode resolution did not return a mode map"
fi

transition_audit_type="$(yq -r '.transitionAudit | type' "$resolved_mode_file" 2>/dev/null || true)"
if [[ "$transition_audit_type" != "!!null" && "$transition_audit_type" != "!!map" ]]; then
  fail 72 E009-AUDIT-PROFILE-CONTRADICTION "transitionAudit must be a map when present"
fi
if [[ "$transition_audit_type" == "!!map" ]]; then
  transition_audit_keys="$(yq -o=json -I=0 '.transitionAudit | keys | sort' "$resolved_mode_file" 2>/dev/null || true)"
  if [[ "$transition_audit_keys" != '["profile","target"]' ]]; then
    if [[ "$(yq -r '.transitionAudit | has("profile")' "$resolved_mode_file" 2>/dev/null || true)" != "true" ]]; then
      fail 70 E009-AUDIT-PROFILE-MISSING "transitionAudit has no profile binding"
    fi
    fail 72 E009-AUDIT-PROFILE-CONTRADICTION "transitionAudit contains missing or unsupported fields"
  fi
  profile_type="$(yq -r '.transitionAudit.profile | type' "$resolved_mode_file" 2>/dev/null || true)"
  target_type="$(yq -r '.transitionAudit.target | type' "$resolved_mode_file" 2>/dev/null || true)"
  if [[ "$profile_type" != "!!str" || "$target_type" != "!!str" ]]; then
    fail 72 E009-AUDIT-PROFILE-CONTRADICTION "transitionAudit requires string profile and target fields"
  fi
fi

status_ceiling="$(yq -r '.statusCeiling // ""' "$resolved_mode_file")"
audit_profile="$(yq -r '.transitionAudit.profile // ""' "$resolved_mode_file")"
transition_target="$(yq -r '.transitionAudit.target // ""' "$resolved_mode_file")"
focus="$(yq -r '.constraints.focus // ""' "$resolved_mode_file")"
allow_implementation_json="$(yq -o=json -I=0 '.constraints.allowImplementationForFindings' "$resolved_mode_file")"
mode_class_json="$(yq -o=json -I=0 '.constraints.modeClass' "$resolved_mode_file")"
phase_order_json="$(yq -o=json -I=0 '.phaseOrder // []' "$resolved_mode_file")"
required_gates_json="$(yq -o=json -I=0 '.requiredGates // []' "$resolved_mode_file")"

if [[ -z "$status_ceiling" \
  || "$(printf '%s' "$phase_order_json" | jq -r 'type')" != "array" \
  || "$(printf '%s' "$required_gates_json" | jq -r 'type')" != "array" \
  || "$(printf '%s' "$phase_order_json" | jq -r 'all(.[]; type == "string")')" != "true" \
  || "$(printf '%s' "$required_gates_json" | jq -r 'all(.[]; type == "string" and test("^G[0-9]{3}$"))')" != "true" ]]; then
  fail 72 E009-AUDIT-PROFILE-CONTRADICTION "resolved mode lacks a valid ceiling, phase order, or gate set"
fi
mode_class_type="$(printf '%s' "$mode_class_json" | jq -r 'type')"
if [[ "$mode_class_type" != "null" && "$mode_class_type" != "string" ]]; then
  fail 72 E009-AUDIT-PROFILE-CONTRADICTION "resolved modeClass must be a string or null"
fi

has_phase() {
  printf '%s' "$phase_order_json" | jq -e --arg phase "$1" 'index($phase) != null' >/dev/null
}

has_gate() {
  printf '%s' "$required_gates_json" | jq -e --arg gate "$1" 'index($gate) != null' >/dev/null
}

if [[ -z "$audit_profile" ]]; then
  if [[ "$workflow_mode" == "product-to-planning" \
    || "$workflow_mode" == "spec-scope-hardening" \
    || ( "$status_ceiling" == "done" && "$(printf '%s' "$phase_order_json" | jq -r 'index("audit") != null')" == "true" ) ]]; then
    fail 70 E009-AUDIT-PROFILE-MISSING "registry-bound audit mode has no explicit transition audit profile"
  fi
  fail 71 E009-AUDIT-PROFILE-UNSUPPORTED "resolved mode has no supported transition audit contract"
fi
if [[ "$audit_profile" != "planning-maturity-v1" && "$audit_profile" != "delivery-completion-v1" ]]; then
  fail 71 E009-AUDIT-PROFILE-UNSUPPORTED "resolved mode declares an unknown transition audit profile"
fi
if [[ "$transition_target" != "statusCeiling" ]]; then
  fail 72 E009-AUDIT-PROFILE-CONTRADICTION "transition audit target must be statusCeiling"
fi

source_edit_lockout_required=false
if has_gate G073; then
  source_edit_lockout_required=true
fi

if [[ "$audit_profile" == "planning-maturity-v1" ]]; then
  if [[ "$status_ceiling" != "specs_hardened" \
    || "$allow_implementation_json" != "false" \
    || ( "$focus" != "planning_only" && "$focus" != "specs_and_scopes_only" ) \
    || "$source_edit_lockout_required" != "true" ]] \
    || ! has_phase validate \
    || ! has_phase audit \
    || ! has_phase finalize \
    || has_phase implement \
    || has_phase test; then
    fail 72 E009-AUDIT-PROFILE-CONTRADICTION "planning profile invariants contradict the resolved mode"
  fi
else
  if [[ "$status_ceiling" != "done" ]] || ! has_phase validate || ! has_phase audit; then
    fail 72 E009-AUDIT-PROFILE-CONTRADICTION "delivery profile ceiling or required phases contradict the resolved mode"
  fi
  case "$workflow_mode" in
    chaos-to-doc|redteam-to-doc)
      ;;
    test-to-doc|simplify-to-doc|devops-to-doc|retro-to-simplify)
      if ! has_phase test; then
        fail 72 E009-AUDIT-PROFILE-CONTRADICTION "delivery compatibility mode is missing its required test phase"
      fi
      ;;
    *)
      if ! has_phase implement || ! has_phase test; then
        fail 72 E009-AUDIT-PROFILE-CONTRADICTION "delivery profile is missing implement or test"
      fi
      ;;
  esac
fi

target_status="$status_ceiling"
case "$current_status" in
  not_started|in_progress|blocked|"$target_status")
    ;;
  *)
    fail 69 E009-TARGET-MISMATCH "current terminal state contradicts the registry-derived target"
    ;;
esac
if [[ "$audit_profile" == "planning-maturity-v1" && "$current_status" == "done" ]]; then
  fail 69 E009-TARGET-MISMATCH "planning profile cannot be paired with done status"
fi

contract_projection="$(jq -cnS \
  --arg workflowMode "$workflow_mode" \
  --arg auditProfile "$audit_profile" \
  --arg statusCeiling "$status_ceiling" \
  --arg targetStatus "$target_status" \
  --arg transitionTarget "$transition_target" \
  --arg focus "$focus" \
  --argjson requiredGates "$required_gates_json" \
  --argjson phaseOrder "$phase_order_json" \
  --argjson allowImplementationForFindings "$allow_implementation_json" \
  --argjson sourceEditLockoutRequired "$source_edit_lockout_required" \
  '{
    workflowMode: $workflowMode,
    auditProfile: $auditProfile,
    statusCeiling: $statusCeiling,
    targetStatus: $targetStatus,
    requiredGates: $requiredGates,
    phaseOrder: $phaseOrder,
    profileValidation: {
      transitionTarget: $transitionTarget,
      allowImplementationForFindings: $allowImplementationForFindings,
      focus: $focus,
      sourceEditLockoutRequired: $sourceEditLockoutRequired
    }
  }')"
contract_digest="sha256:$(printf '%s' "$contract_projection" | bubbles_sha256_stdin)"

target_manifest="$tmp_dir/target-revision.manifest"
: > "$target_manifest"

append_digest_record() {
  local label="$1"
  local target_file="$2"
  local digest
  if [[ -f "$target_file" ]]; then
    digest="$(bubbles_sha256_file "$target_file")"
  else
    digest="MISSING"
  fi
  printf '%s\t%s\n' "$label" "$digest" >> "$target_manifest"
}

append_report_record() {
  local label="$1"
  local report_file="$2"
  local digest
  digest="$(awk '
    /^[[:space:]]*BEGIN AUDIT_RESULT_V1[[:space:]]*$/ { in_audit=1; next }
    /^[[:space:]]*END AUDIT_RESULT_V1[[:space:]]*$/ { in_audit=0; next }
    !in_audit { print }
  ' "$report_file" | bubbles_sha256_stdin)"
  printf '%s\t%s\n' "$label" "$digest" >> "$target_manifest"
}

append_digest_record spec.md "$feature_dir/spec.md"
append_digest_record design.md "$feature_dir/design.md"
append_digest_record uservalidation.md "$feature_dir/uservalidation.md"
for optional_artifact in scenario-manifest.json test-plan.json; do
  if [[ -f "$feature_dir/$optional_artifact" ]]; then
    append_digest_record "$optional_artifact" "$feature_dir/$optional_artifact"
  fi
done

if [[ -f "$feature_dir/scopes.md" ]]; then
  append_digest_record scopes.md "$feature_dir/scopes.md"
fi
if [[ -d "$feature_dir/scopes" ]]; then
  while IFS= read -r scope_file; do
    append_digest_record "${scope_file#"$feature_dir"/}" "$scope_file"
  done < <(find "$feature_dir/scopes" -mindepth 2 -maxdepth 2 -type f -name scope.md | LC_ALL=C sort)
fi
if [[ -f "$feature_dir/report.md" ]]; then
  append_report_record report.md "$feature_dir/report.md"
fi
if [[ -d "$feature_dir/scopes" ]]; then
  while IFS= read -r report_file; do
    append_report_record "${report_file#"$feature_dir"/}" "$report_file"
  done < <(find "$feature_dir/scopes" -mindepth 2 -maxdepth 2 -type f -name report.md | LC_ALL=C sort)
fi

state_projection_file="$tmp_dir/state-projection.json"
if ! jq -cS '
  del(.lastUpdatedAt)
  | if ((.execution // null) | type) == "object" then
      .execution |= (
        del(.audit)
        | if .currentPhase == "audit" then del(.currentPhase) else . end
        | if ((.executionHistory // null) | type) == "array" then
            .executionHistory |= map(select((.phase // "") != "audit" and (.agent // "") != "bubbles.audit"))
          else . end
      )
    else . end
  | if .currentPhase == "audit" then del(.currentPhase) else . end
  | if ((.executionHistory // null) | type) == "array" then
      .executionHistory |= map(select((.phase // "") != "audit" and (.agent // "") != "bubbles.audit"))
    else . end
' "$state_file" > "$state_projection_file"; then
  fail 65 E009-STATE-MALFORMED "state.json cannot be projected canonically"
fi
append_digest_record state.json#canonical "$state_projection_file"
target_revision="sha256:$(bubbles_sha256_file "$target_manifest")"

if [[ -n "$expect_mode" && "$expect_mode" != "$workflow_mode" ]]; then
  fail 69 E009-TARGET-MISMATCH "expected workflow mode does not match the derived contract"
fi
if [[ -n "$expect_target" && "$expect_target" != "$target_status" ]]; then
  fail 69 E009-TARGET-MISMATCH "expected target does not match the derived contract"
fi
if [[ -n "$expect_contract_digest" && "$expect_contract_digest" != "$contract_digest" ]]; then
  fail 69 E009-TARGET-MISMATCH "expected contract digest does not match the derived contract"
fi

contract_ref="bubbles/workflows/modes.yaml#$workflow_mode"
jq -cn \
  --arg schemaVersion transition-contract/v1 \
  --arg featureDir "$feature_display" \
  --arg workflowMode "$workflow_mode" \
  --arg auditProfile "$audit_profile" \
  --arg statusCeiling "$status_ceiling" \
  --arg targetStatus "$target_status" \
  --arg currentStatus "$current_status" \
  --arg contractRef "$contract_ref" \
  --arg contractDigest "$contract_digest" \
  --arg targetRevision "$target_revision" \
  --argjson modeClass "$mode_class_json" \
  --argjson requiredGates "$required_gates_json" \
  --argjson phaseOrder "$phase_order_json" \
  --argjson sourceEditLockoutRequired "$source_edit_lockout_required" \
  '{
    schemaVersion: $schemaVersion,
    featureDir: $featureDir,
    workflowMode: $workflowMode,
    modeClass: $modeClass,
    auditProfile: $auditProfile,
    statusCeiling: $statusCeiling,
    targetStatus: $targetStatus,
    currentStatus: $currentStatus,
    requiredGates: $requiredGates,
    phaseOrder: $phaseOrder,
    sourceEditLockoutRequired: $sourceEditLockoutRequired,
    contractRef: $contractRef,
    contractDigest: $contractDigest,
    targetRevision: $targetRevision
  }'