#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
FRAMEWORK_DIR="$REPO_ROOT/bubbles"
RESOLVER="$SCRIPT_DIR/transition-contract-resolver.sh"
MODE_RESOLVER="$SCRIPT_DIR/mode-resolver.sh"
GUARD_LIB="$SCRIPT_DIR/guard-lib.sh"
SCHEMA="$FRAMEWORK_DIR/schemas/workflows.schema.json"

for required_file in "$RESOLVER" "$MODE_RESOLVER" "$GUARD_LIB" "$SCHEMA"; do
  if [[ ! -f "$required_file" ]]; then
    printf 'transition-contract-resolver-selftest: required surface missing: %s\n' "$required_file" >&2
    exit 2
  fi
done
for required_command in jq yq; do
  if ! command -v "$required_command" >/dev/null 2>&1; then
    printf 'transition-contract-resolver-selftest: required command missing: %s\n' "$required_command" >&2
    exit 2
  fi
done

# shellcheck source=/dev/null
source "$GUARD_LIB"

WORKSPACE="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-transition-contract-selftest.XXXXXX")"
cleanup() {
  rm -rf "$WORKSPACE"
}
trap cleanup EXIT INT TERM

PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0

pass() {
  PASS_COUNT=$((PASS_COUNT + 1))
  printf 'PASS: %s\n' "$1"
}

fail_test() {
  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf 'FAIL: %s\n' "$1" >&2
}

skip_test() {
  SKIP_COUNT=$((SKIP_COUNT + 1))
  printf 'SKIP: %s\n' "$1"
}

assert_equal() {
  local expected="$1"
  local actual="$2"
  local label="$3"
  if [[ "$expected" == "$actual" ]]; then
    pass "$label"
  else
    fail_test "$label (expected '$expected', observed '$actual')"
  fi
}

assert_files_equal() {
  local expected_file="$1"
  local actual_file="$2"
  local label="$3"
  if cmp -s "$expected_file" "$actual_file"; then
    pass "$label"
  else
    fail_test "$label (files differ)"
  fi
}

assert_json() {
  local json_file="$1"
  local expression="$2"
  local label="$3"
  if jq -e "$expression" "$json_file" >/dev/null 2>&1; then
    pass "$label"
  else
    fail_test "$label (jq assertion failed: $expression)"
  fi
}

assert_failure() {
  local label="$1"
  local expected_exit="$2"
  local expected_code="$3"
  shift 3
  local safe_label
  local stdout_file
  local stderr_file
  local exit_code
  safe_label="$(printf '%s' "$label" | tr ' /' '__')"
  stdout_file="$WORKSPACE/$safe_label.stdout"
  stderr_file="$WORKSPACE/$safe_label.stderr"

  set +e
  "$@" > "$stdout_file" 2> "$stderr_file"
  exit_code=$?
  set -e

  if [[ "$exit_code" -ne "$expected_exit" ]]; then
    fail_test "$label exits $expected_exit (observed $exit_code)"
    return
  fi
  if [[ -s "$stdout_file" ]]; then
    fail_test "$label emits no usable contract on failure"
    return
  fi
  if [[ "$(wc -l < "$stderr_file" | tr -d ' ')" != "1" ]] \
    || ! grep -Eq "^${expected_code}: [^[:space:]].*$" "$stderr_file"; then
    fail_test "$label emits one stable $expected_code line"
    return
  fi
  pass "$label returns $expected_code with exit $expected_exit and empty stdout"
}

write_feature() {
  local feature_dir="$1"
  local workflow_mode="$2"
  local current_status="${3:-in_progress}"
  mkdir -p "$feature_dir/scopes/S01-foundation"

  cat <<'EOF' > "$feature_dir/spec.md"
# Transition Contract Fixture

## Requirement

Resolve a registry-bound audit contract without selecting a profile at the caller.
EOF
  cat <<'EOF' > "$feature_dir/design.md"
# Transition Contract Fixture Design

The canonical workflow registry is the sole transition audit policy source.
EOF
  cat <<'EOF' > "$feature_dir/uservalidation.md"
# User Validation

- [x] The target and profile are visible in one normalized contract.
EOF
  cat <<'EOF' > "$feature_dir/scenario-manifest.json"
{"version":1,"scenarios":[]}
EOF
  cat <<'EOF' > "$feature_dir/test-plan.json"
{"version":1,"tests":[]}
EOF
  cat <<'EOF' > "$feature_dir/scopes/S01-foundation/scope.md"
# Scope S01 Foundation

**Status:** Not Started

### Definition of Done

- [ ] The planned transition contract is implemented.
EOF
  cat <<'EOF' > "$feature_dir/scopes/S01-foundation/report.md"
# Scope Report

No implementation evidence is claimed by this resolver fixture.
EOF
  cat <<EOF > "$feature_dir/state.json"
{
  "version": 3,
  "status": "$current_status",
  "workflowMode": "$workflow_mode",
  "execution": {
    "currentPhase": "audit",
    "completedPhaseClaims": ["analyze", "ux", "design", "plan"]
  },
  "certification": {
    "status": "$current_status",
    "completedScopes": []
  },
  "policySnapshot": {
    "workflowMode": "$workflow_mode"
  },
  "executionHistory": [
    {
      "phase": "plan",
      "agent": "bubbles.plan",
      "outcome": "completed_diagnostic"
    }
  ],
  "lastUpdatedAt": "2026-07-10T12:00:00Z"
}
EOF
}

set_state_mode() {
  local feature_dir="$1"
  local workflow_mode="$2"
  local state_file="$feature_dir/state.json"
  local temp_file="$WORKSPACE/state-mode.json"
  jq --arg mode "$workflow_mode" \
    '.workflowMode = $mode | .policySnapshot.workflowMode = $mode' \
    "$state_file" > "$temp_file"
  mv "$temp_file" "$state_file"
}

copy_framework_layout() {
  local layout_kind="$1"
  local layout_root="$2"
  local destination
  case "$layout_kind" in
    source)
      destination="$layout_root/bubbles"
      ;;
    installed)
      destination="$layout_root/.github/bubbles"
      ;;
    *)
      fail_test "unknown framework layout: $layout_kind"
      return 1
      ;;
  esac

  mkdir -p "$destination/scripts" "$destination/workflows"
  cp "$FRAMEWORK_DIR/workflows.yaml" "$destination/workflows.yaml"
  cp "$FRAMEWORK_DIR/workflows/modes.yaml" "$destination/workflows/modes.yaml"
  cp "$FRAMEWORK_DIR/workflows/aliases.yaml" "$destination/workflows/aliases.yaml"
  cp "$SCRIPT_DIR/transition-contract-resolver.sh" "$destination/scripts/transition-contract-resolver.sh"
  cp "$SCRIPT_DIR/mode-resolver.sh" "$destination/scripts/mode-resolver.sh"
  cp "$SCRIPT_DIR/guard-lib.sh" "$destination/scripts/guard-lib.sh"
  cp "$SCRIPT_DIR/trust-metadata.sh" "$destination/scripts/trust-metadata.sh"
  printf '%s' "$destination"
}

schema_contract_tests() {
  if jq -e '
    .properties.modes.patternProperties["^[a-z][a-z0-9-]*$"].properties.transitionAudit as $contract
    | $contract.type == "object"
      and $contract.required == ["profile", "target"]
      and $contract.additionalProperties == false
      and (($contract.properties.profile.enum | sort) == (["delivery-completion-v1", "planning-maturity-v1"] | sort))
      and $contract.properties.target.const == "statusCeiling"
  ' "$SCHEMA" >/dev/null 2>&1; then
    pass "transitionAudit schema is closed to the designed profile and target fields"
  else
    fail_test "transitionAudit schema is closed to the designed profile and target fields"
  fi

  if command -v python3 >/dev/null 2>&1 \
    && python3 -c 'import yaml, jsonschema' >/dev/null 2>&1; then
    if python3 - "$FRAMEWORK_DIR/workflows.yaml" "$FRAMEWORK_DIR/workflows/modes.yaml" "$SCHEMA" <<'PY'
import copy
import json
import sys

import yaml
from jsonschema import Draft7Validator

with open(sys.argv[1], encoding="utf-8") as handle:
    workflows = yaml.safe_load(handle)
with open(sys.argv[2], encoding="utf-8") as handle:
    modes = yaml.safe_load(handle)
with open(sys.argv[3], encoding="utf-8") as handle:
    schema = json.load(handle)

document = dict(workflows)
document.update(modes)
validator = Draft7Validator(schema)
validator.validate(document)

mutations = []
unknown_profile = copy.deepcopy(document)
unknown_profile["modes"]["product-to-planning"]["transitionAudit"]["profile"] = "caller-selected-v9"
mutations.append(unknown_profile)
unknown_target = copy.deepcopy(document)
unknown_target["modes"]["product-to-planning"]["transitionAudit"]["target"] = "done"
mutations.append(unknown_target)
extra_field = copy.deepcopy(document)
extra_field["modes"]["product-to-planning"]["transitionAudit"]["profileSelector"] = "caller"
mutations.append(extra_field)

for mutation in mutations:
    if not list(validator.iter_errors(mutation)):
        raise SystemExit("closed transitionAudit schema accepted an invalid mutation")
PY
    then
      pass "schema accepts canonical bindings and rejects unknown profile, target, and selector fields"
    else
      fail_test "closed transitionAudit schema behavior"
    fi
    return
  fi

  if command -v jsonschema >/dev/null 2>&1; then
    local canonical_registry="$WORKSPACE/schema-canonical.json"
    local unknown_profile="$WORKSPACE/schema-unknown-profile.json"
    local unknown_target="$WORKSPACE/schema-unknown-target.json"
    local extra_field="$WORKSPACE/schema-extra-field.json"
    local validator_output="$WORKSPACE/schema-validator.out"

    # $item is a yq variable, not a shell variable.
    # shellcheck disable=SC2016
    yq eval-all -o=json -I=0 '. as $item ireduce ({}; . * $item)' \
      "$FRAMEWORK_DIR/workflows.yaml" "$FRAMEWORK_DIR/workflows/modes.yaml" > "$canonical_registry"
    jq '.modes["product-to-planning"].transitionAudit.profile = "caller-selected-v9"' \
      "$canonical_registry" > "$unknown_profile"
    jq '.modes["product-to-planning"].transitionAudit.target = "done"' \
      "$canonical_registry" > "$unknown_target"
    jq '.modes["product-to-planning"].transitionAudit.profileSelector = "caller"' \
      "$canonical_registry" > "$extra_field"

    if jsonschema -i "$canonical_registry" "$SCHEMA" > "$validator_output" 2>&1 \
      && ! jsonschema -i "$unknown_profile" "$SCHEMA" >> "$validator_output" 2>&1 \
      && ! jsonschema -i "$unknown_target" "$SCHEMA" >> "$validator_output" 2>&1 \
      && ! jsonschema -i "$extra_field" "$SCHEMA" >> "$validator_output" 2>&1; then
      pass "schema accepts canonical bindings and rejects unknown profile, target, and selector fields"
    else
      fail_test "closed transitionAudit schema behavior"
    fi
    return
  fi

  skip_test "JSON Schema engine mutation checks (no validator installed)"
}

printf '%s\n' '== transition contract resolver selftest =='

schema_contract_tests

planning_feature="$WORKSPACE/specs/001-product-planning"
hardening_feature="$WORKSPACE/specs/002-scope-hardening"
delivery_feature="$WORKSPACE/specs/003-delivery"
write_feature "$planning_feature" product-to-planning
write_feature "$hardening_feature" spec-scope-hardening
write_feature "$delivery_feature" bugfix-fastlane

planning_contract="$WORKSPACE/planning-contract.json"
planning_repeat="$WORKSPACE/planning-repeat.json"
hardening_contract="$WORKSPACE/hardening-contract.json"
delivery_contract="$WORKSPACE/delivery-contract.json"

if bash "$RESOLVER" "$planning_feature" > "$planning_contract"; then
  pass "product-to-planning resolves through the production resolver"
else
  fail_test "product-to-planning resolves through the production resolver"
fi
if bash "$RESOLVER" "$hardening_feature" > "$hardening_contract"; then
  pass "spec-scope-hardening resolves through the production resolver"
else
  fail_test "spec-scope-hardening resolves through the production resolver"
fi
if bash "$RESOLVER" "$delivery_feature" > "$delivery_contract"; then
  pass "bugfix-fastlane resolves a delivery contract"
else
  fail_test "bugfix-fastlane resolves a delivery contract"
fi

assert_json "$planning_contract" '.schemaVersion == "transition-contract/v1" and .featureDir == "specs/001-product-planning"' "planning contract has normalized schema and feature path"
assert_json "$planning_contract" '.workflowMode == "product-to-planning" and .modeClass == null' "planning contract names the canonical persisted mode"
assert_json "$planning_contract" '.auditProfile == "planning-maturity-v1" and .statusCeiling == "specs_hardened" and .targetStatus == "specs_hardened"' "planning contract derives profile and target from the registry"
assert_json "$planning_contract" '.currentStatus == "in_progress" and .sourceEditLockoutRequired == true' "planning contract exposes current state and G073 source lockout"
assert_json "$planning_contract" '.contractRef == "bubbles/workflows/modes.yaml#product-to-planning"' "planning contract carries the canonical registry reference"
assert_json "$planning_contract" '(.contractDigest | test("^sha256:[0-9a-f]{64}$")) and (.targetRevision | test("^sha256:[0-9a-f]{64}$"))' "planning contract carries deterministic SHA-256 identities"
assert_json "$hardening_contract" '.workflowMode == "spec-scope-hardening" and .auditProfile == "planning-maturity-v1" and .targetStatus == "specs_hardened" and .sourceEditLockoutRequired == true' "scope hardening satisfies the planning profile invariants"
assert_json "$delivery_contract" '.workflowMode == "bugfix-fastlane" and .auditProfile == "delivery-completion-v1" and .targetStatus == "done"' "delivery mode retains explicit completion semantics"

expected_planning_mode="$WORKSPACE/product-to-planning.resolved.yaml"
bash "$MODE_RESOLVER" --grandfather product-to-planning > "$expected_planning_mode" 2> "$WORKSPACE/product-to-planning.resolved.err"
expected_gates="$(yq -o=json -I=0 '.requiredGates' "$expected_planning_mode" | jq -cS '.')"
actual_gates="$(jq -cS '.requiredGates' "$planning_contract")"
assert_equal "$expected_gates" "$actual_gates" "resolver emits the complete sorted effective gate set"
expected_phases="$(yq -o=json -I=0 '.phaseOrder' "$expected_planning_mode" | jq -c '.')"
actual_phases="$(jq -c '.phaseOrder' "$planning_contract")"
assert_equal "$expected_phases" "$actual_phases" "resolver preserves the complete ordered phase list"

alias_mode="$(bash "$MODE_RESOLVER" --resolve-v6 plan target:product action:analyze-design-plan)"
assert_equal product-to-planning "$alias_mode" "v6 planning form maps to the persisted canonical key"
v5_resolved="$WORKSPACE/v5-planning.yaml"
v6_resolved="$WORKSPACE/v6-planning.yaml"
bash "$MODE_RESOLVER" --grandfather product-to-planning > "$v5_resolved" 2> "$WORKSPACE/v5-planning.err"
bash "$MODE_RESOLVER" plan target:product action:analyze-design-plan > "$v6_resolved"
assert_files_equal "$v5_resolved" "$v6_resolved" "persisted v5 and current v6 forms resolve byte-identical mode definitions"
set_state_mode "$planning_feature" "$alias_mode"
bash "$RESOLVER" "$planning_feature" > "$planning_repeat"
assert_files_equal "$planning_contract" "$planning_repeat" "persisted and v6-derived canonical modes produce byte-identical transition contracts"

bash "$RESOLVER" "$planning_feature" > "$planning_repeat"
assert_files_equal "$planning_contract" "$planning_repeat" "repeated resolution is byte-stable"

contract_digest="$(jq -r '.contractDigest' "$planning_contract")"
target_revision="$(jq -r '.targetRevision' "$planning_contract")"
assert_equal "$contract_digest" "$(jq -r '.contractDigest' "$planning_repeat")" "contract digest is stable across idempotent resolution"
assert_equal "$target_revision" "$(jq -r '.targetRevision' "$planning_repeat")" "target revision is stable across idempotent resolution"

matching_assertions="$WORKSPACE/matching-assertions.json"
bash "$RESOLVER" "$planning_feature" \
  --expect-mode product-to-planning \
  --expect-target specs_hardened \
  --expect-contract-digest "$contract_digest" > "$matching_assertions"
assert_files_equal "$planning_contract" "$matching_assertions" "expectation flags only confirm and never alter the derived contract"

assert_failure "expected mode mismatch" 69 E009-TARGET-MISMATCH \
  bash "$RESOLVER" "$planning_feature" --expect-mode spec-scope-hardening
assert_failure "expected target mismatch" 69 E009-TARGET-MISMATCH \
  bash "$RESOLVER" "$planning_feature" --expect-target "done"
assert_failure "stale digest mismatch" 69 E009-TARGET-MISMATCH \
  bash "$RESOLVER" "$planning_feature" --expect-contract-digest "sha256:$(printf '0%.0s' {1..64})"
assert_failure "caller profile flag" 64 E009-USAGE \
  bash "$RESOLVER" "$planning_feature" --profile planning-maturity-v1
assert_failure "caller bypass flag" 64 E009-USAGE \
  bash "$RESOLVER" "$planning_feature" --force
assert_failure "caller profile environment" 64 E009-USAGE \
  env AUDIT_PROFILE=planning-maturity-v1 bash "$RESOLVER" "$planning_feature"
assert_failure "missing feature argument" 64 E009-USAGE bash "$RESOLVER"

audit_state_temp="$WORKSPACE/audit-state.json"
jq '
  .execution.audit = {schemaVersion: "audit-run/v1", attemptId: "attempt-2"}
  | .execution.currentPhase = "audit"
  | .executionHistory += [{phase: "audit", agent: "bubbles.audit", outcome: "completed_diagnostic"}]
  | .lastUpdatedAt = "2026-07-10T13:00:00Z"
' "$planning_feature/state.json" > "$audit_state_temp"
mv "$audit_state_temp" "$planning_feature/state.json"
cat <<'EOF' >> "$planning_feature/scopes/S01-foundation/report.md"
BEGIN AUDIT_RESULT_V1
attemptId: attempt-2
verdict: PLANNING_AUDIT_CLEAN
END AUDIT_RESULT_V1
EOF
audit_owned_repeat="$WORKSPACE/audit-owned-repeat.json"
bash "$RESOLVER" "$planning_feature" > "$audit_owned_repeat"
assert_files_equal "$planning_contract" "$audit_owned_repeat" "audit-owned state and report blocks do not invalidate their own target revision"

printf '%s\n' 'A non-audit planning artifact changed.' >> "$planning_feature/design.md"
foreign_mutation_contract="$WORKSPACE/foreign-mutation-contract.json"
bash "$RESOLVER" "$planning_feature" > "$foreign_mutation_contract"
assert_equal "$contract_digest" "$(jq -r '.contractDigest' "$foreign_mutation_contract")" "artifact mutation does not change the registry contract digest"
if [[ "$target_revision" != "$(jq -r '.targetRevision' "$foreign_mutation_contract")" ]]; then
  pass "non-audit artifact mutation changes the target revision"
else
  fail_test "non-audit artifact mutation changes the target revision"
fi

source_layout_root="$WORKSPACE/source-layout"
installed_layout_root="$WORKSPACE/installed-layout"
source_framework="$(copy_framework_layout source "$source_layout_root")"
installed_framework="$(copy_framework_layout installed "$installed_layout_root")"
source_layout_contract="$WORKSPACE/source-layout-contract.json"
installed_layout_contract="$WORKSPACE/installed-layout-contract.json"
bash "$source_framework/scripts/transition-contract-resolver.sh" "$delivery_feature" > "$source_layout_contract"
bash "$installed_framework/scripts/transition-contract-resolver.sh" "$delivery_feature" > "$installed_layout_contract"
assert_files_equal "$delivery_contract" "$source_layout_contract" "source layout resolves byte-identical contracts"
assert_files_equal "$delivery_contract" "$installed_layout_contract" "installed .github/bubbles layout resolves byte-identical contracts"

missing_registry_root="$WORKSPACE/missing-registry-layout"
missing_registry_framework="$(copy_framework_layout installed "$missing_registry_root")"
rm "$missing_registry_framework/workflows/modes.yaml"
assert_failure "missing registry" 66 E009-REGISTRY-MISSING \
  bash "$missing_registry_framework/scripts/transition-contract-resolver.sh" "$planning_feature"

malformed_feature="$WORKSPACE/specs/010-malformed"
mkdir -p "$malformed_feature"
printf '%s\n' '{not-json' > "$malformed_feature/state.json"
assert_failure "malformed state" 65 E009-STATE-MALFORMED bash "$RESOLVER" "$malformed_feature"

unknown_feature="$WORKSPACE/specs/011-unknown-mode"
write_feature "$unknown_feature" custom-planning-audit
assert_failure "unknown mode" 67 E009-MODE-UNKNOWN bash "$RESOLVER" "$unknown_feature"

mode_mismatch_feature="$WORKSPACE/specs/012-mode-mismatch"
write_feature "$mode_mismatch_feature" product-to-planning
mode_mismatch_temp="$WORKSPACE/mode-mismatch.json"
jq '.policySnapshot.workflowMode = "full-delivery"' "$mode_mismatch_feature/state.json" > "$mode_mismatch_temp"
mv "$mode_mismatch_temp" "$mode_mismatch_feature/state.json"
assert_failure "state policy mode mismatch" 68 E009-STATE-MODE-MISMATCH bash "$RESOLVER" "$mode_mismatch_feature"

status_mismatch_feature="$WORKSPACE/specs/013-status-mismatch"
write_feature "$status_mismatch_feature" product-to-planning
status_mismatch_temp="$WORKSPACE/status-mismatch.json"
jq '.certification.status = "blocked"' "$status_mismatch_feature/state.json" > "$status_mismatch_temp"
mv "$status_mismatch_temp" "$status_mismatch_feature/state.json"
assert_failure "certification mirror mismatch" 69 E009-TARGET-MISMATCH bash "$RESOLVER" "$status_mismatch_feature"

terminal_mismatch_feature="$WORKSPACE/specs/014-terminal-mismatch"
write_feature "$terminal_mismatch_feature" product-to-planning docs_updated
assert_failure "terminal target mismatch" 69 E009-TARGET-MISMATCH bash "$RESOLVER" "$terminal_mismatch_feature"

missing_profile_root="$WORKSPACE/missing-profile-layout"
missing_profile_framework="$(copy_framework_layout installed "$missing_profile_root")"
yq -i 'del(.modes.bugfix-fastlane.transitionAudit)' "$missing_profile_framework/workflows/modes.yaml"
assert_failure "missing delivery profile" 70 E009-AUDIT-PROFILE-MISSING \
  bash "$missing_profile_framework/scripts/transition-contract-resolver.sh" "$delivery_feature"

missing_planning_profile_root="$WORKSPACE/missing-planning-profile-layout"
missing_planning_profile_framework="$(copy_framework_layout installed "$missing_planning_profile_root")"
yq -i 'del(.modes.product-to-planning.transitionAudit)' "$missing_planning_profile_framework/workflows/modes.yaml"
assert_failure "missing designated planning profile" 70 E009-AUDIT-PROFILE-MISSING \
  bash "$missing_planning_profile_framework/scripts/transition-contract-resolver.sh" "$planning_feature"

unsupported_feature="$WORKSPACE/specs/015-unsupported"
write_feature "$unsupported_feature" docs-only
assert_failure "unsupported adjacent non-done mode" 71 E009-AUDIT-PROFILE-UNSUPPORTED \
  bash "$RESOLVER" "$unsupported_feature"

unknown_profile_root="$WORKSPACE/unknown-profile-layout"
unknown_profile_framework="$(copy_framework_layout installed "$unknown_profile_root")"
yq -i '.modes.product-to-planning.transitionAudit.profile = "future-profile-v9"' "$unknown_profile_framework/workflows/modes.yaml"
assert_failure "unknown explicit profile" 71 E009-AUDIT-PROFILE-UNSUPPORTED \
  bash "$unknown_profile_framework/scripts/transition-contract-resolver.sh" "$planning_feature"

malformed_profile_root="$WORKSPACE/malformed-profile-layout"
malformed_profile_framework="$(copy_framework_layout installed "$malformed_profile_root")"
yq -i '.modes.product-to-planning.transitionAudit = "planning-maturity-v1"' "$malformed_profile_framework/workflows/modes.yaml"
assert_failure "malformed transition audit metadata" 72 E009-AUDIT-PROFILE-CONTRADICTION \
  bash "$malformed_profile_framework/scripts/transition-contract-resolver.sh" "$planning_feature"

contradictory_planning_root="$WORKSPACE/contradictory-planning-layout"
contradictory_planning_framework="$(copy_framework_layout installed "$contradictory_planning_root")"
yq -i '.modes.product-to-planning.phaseOrder += ["implement"]' "$contradictory_planning_framework/workflows/modes.yaml"
assert_failure "planning implementation phase contradiction" 72 E009-AUDIT-PROFILE-CONTRADICTION \
  bash "$contradictory_planning_framework/scripts/transition-contract-resolver.sh" "$planning_feature"

contradictory_target_root="$WORKSPACE/contradictory-target-layout"
contradictory_target_framework="$(copy_framework_layout installed "$contradictory_target_root")"
yq -i '.modes.product-to-planning.transitionAudit.target = "done"' "$contradictory_target_framework/workflows/modes.yaml"
assert_failure "registry target contradiction" 72 E009-AUDIT-PROFILE-CONTRADICTION \
  bash "$contradictory_target_framework/scripts/transition-contract-resolver.sh" "$planning_feature"

extra_metadata_root="$WORKSPACE/extra-metadata-layout"
extra_metadata_framework="$(copy_framework_layout installed "$extra_metadata_root")"
yq -i '.modes.product-to-planning.transitionAudit.profileSelector = "caller"' "$extra_metadata_framework/workflows/modes.yaml"
assert_failure "unsupported transition audit field" 72 E009-AUDIT-PROFILE-CONTRADICTION \
  bash "$extra_metadata_framework/scripts/transition-contract-resolver.sh" "$planning_feature"

planning_on_delivery_root="$WORKSPACE/planning-on-delivery-layout"
planning_on_delivery_framework="$(copy_framework_layout installed "$planning_on_delivery_root")"
yq -i '.modes.bugfix-fastlane.transitionAudit.profile = "planning-maturity-v1"' "$planning_on_delivery_framework/workflows/modes.yaml"
assert_failure "planning profile on delivery mode" 72 E009-AUDIT-PROFILE-CONTRADICTION \
  bash "$planning_on_delivery_framework/scripts/transition-contract-resolver.sh" "$delivery_feature"

missing_delivery=0
planning_bindings=0
adjacent_bound=0
adjacent_unbound=0
delivery_compatibility_exceptions=""
while IFS= read -r mode_name; do
  resolved_mode_file="$WORKSPACE/registry-$mode_name.yaml"
  if ! bash "$MODE_RESOLVER" --grandfather "$mode_name" > "$resolved_mode_file" 2> "$WORKSPACE/registry-$mode_name.err"; then
    fail_test "registry inventory resolves $mode_name"
    continue
  fi
  ceiling="$(yq -r '.statusCeiling // ""' "$resolved_mode_file")"
  has_audit="$(yq -r '(.phaseOrder // []) | contains(["audit"])' "$resolved_mode_file")"
  profile="$(yq -r '.transitionAudit.profile // ""' "$resolved_mode_file")"
  if [[ "$ceiling" == "done" && "$has_audit" == "true" && "$profile" != "delivery-completion-v1" ]]; then
    missing_delivery=$((missing_delivery + 1))
  fi
  if [[ "$profile" == "delivery-completion-v1" ]] \
    && { [[ "$(yq -r '(.phaseOrder // []) | contains(["implement"])' "$resolved_mode_file")" != "true" ]] \
      || [[ "$(yq -r '(.phaseOrder // []) | contains(["test"])' "$resolved_mode_file")" != "true" ]]; }; then
    delivery_compatibility_exceptions="${delivery_compatibility_exceptions}${delivery_compatibility_exceptions:+,}$mode_name"
  fi
  if [[ "$profile" == "planning-maturity-v1" ]]; then
    planning_bindings=$((planning_bindings + 1))
  fi
  if [[ "$ceiling" != "done" && "$has_audit" == "true" \
    && "$mode_name" != "product-to-planning" && "$mode_name" != "spec-scope-hardening" ]]; then
    if [[ -n "$profile" ]]; then
      adjacent_bound=$((adjacent_bound + 1))
    else
      adjacent_unbound=$((adjacent_unbound + 1))
    fi
  fi
done < <(bash "$MODE_RESOLVER" --list-modes)
assert_equal 0 "$missing_delivery" "every audit-bearing done mode has an explicit delivery binding"
assert_equal 2 "$planning_bindings" "exactly the two designed planning modes have planning bindings"
assert_equal 0 "$adjacent_bound" "adjacent non-done audit modes receive no inferred profile"
assert_equal 22 "$adjacent_unbound" "all 22 adjacent non-done audit modes remain explicitly unsupported"
delivery_compatibility_exceptions="$(printf '%s\n' "$delivery_compatibility_exceptions" | tr ',' '\n' | LC_ALL=C sort | paste -sd ',' -)"
assert_equal "chaos-to-doc,devops-to-doc,redteam-to-doc,retro-to-simplify,simplify-to-doc,test-to-doc" \
  "$delivery_compatibility_exceptions" \
  "delivery phase-shape compatibility exceptions are a closed six-mode set"

matrix_feature="$WORKSPACE/specs/020-mode-matrix"
write_feature "$matrix_feature" bugfix-fastlane
delivery_matrix_count=0
adjacent_matrix_count=0
while IFS= read -r mode_name; do
  resolved_mode_file="$WORKSPACE/matrix-$mode_name.yaml"
  bash "$MODE_RESOLVER" --grandfather "$mode_name" > "$resolved_mode_file" 2> "$WORKSPACE/matrix-$mode_name.err"
  ceiling="$(yq -r '.statusCeiling // ""' "$resolved_mode_file")"
  has_audit="$(yq -r '(.phaseOrder // []) | contains(["audit"])' "$resolved_mode_file")"
  profile="$(yq -r '.transitionAudit.profile // ""' "$resolved_mode_file")"

  if [[ "$ceiling" == "done" && "$has_audit" == "true" ]]; then
    set_state_mode "$matrix_feature" "$mode_name"
    matrix_output="$WORKSPACE/matrix-$mode_name.json"
    if bash "$RESOLVER" "$matrix_feature" > "$matrix_output" \
      && jq -e --arg mode "$mode_name" \
        '.workflowMode == $mode and .auditProfile == "delivery-completion-v1" and .targetStatus == "done"' \
        "$matrix_output" >/dev/null 2>&1; then
      delivery_matrix_count=$((delivery_matrix_count + 1))
    else
      fail_test "delivery matrix resolves $mode_name with explicit completion semantics"
    fi
  elif [[ "$ceiling" != "done" && "$has_audit" == "true" \
    && "$mode_name" != "product-to-planning" && "$mode_name" != "spec-scope-hardening" ]]; then
    set_state_mode "$matrix_feature" "$mode_name"
    matrix_stdout="$WORKSPACE/matrix-$mode_name.stdout"
    matrix_stderr="$WORKSPACE/matrix-$mode_name.stderr"
    set +e
    bash "$RESOLVER" "$matrix_feature" > "$matrix_stdout" 2> "$matrix_stderr"
    matrix_exit=$?
    set -e
    if [[ "$matrix_exit" -eq 71 && ! -s "$matrix_stdout" ]] \
      && grep -Eq '^E009-AUDIT-PROFILE-UNSUPPORTED: ' "$matrix_stderr"; then
      adjacent_matrix_count=$((adjacent_matrix_count + 1))
    else
      fail_test "adjacent mode $mode_name remains unsupported without ceiling inference"
    fi
  fi
done < <(bash "$MODE_RESOLVER" --list-modes)
assert_equal 27 "$delivery_matrix_count" "all 27 audit-bearing done modes resolve through explicit delivery contracts"
assert_equal 22 "$adjacent_matrix_count" "all 22 adjacent non-done audit modes fail unsupported through the real resolver"

printf '%s\n' '== transition contract resolver selftest summary =='
printf 'passes=%s\nfailures=%s\nskips=%s\n' "$PASS_COUNT" "$FAIL_COUNT" "$SKIP_COUNT"
if (( FAIL_COUNT > 0 )); then
  exit 1
fi
printf '%s\n' 'transition-contract-resolver-selftest: PASS'