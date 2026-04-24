#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source fun mode support
source "$SCRIPT_DIR/fun-mode.sh"

feature_dir="${1:-}"
default_scenario_pattern='SCN-[0-9]{3}-[A-Z][0-9]{2}'
scenario_pattern="$default_scenario_pattern"
enable_autofix="false"

if [[ -z "$feature_dir" ]]; then
  echo "ERROR: missing feature directory argument"
  echo "Usage: bash bubbles/scripts/artifact-lint.sh specs/<NNN-feature-name> [scenario-regex] [--autofix]"
  echo "Example: bash bubbles/scripts/artifact-lint.sh specs/037-future-enhancements-missing-implementation 'SCN-037-[A-Z][0-9]{2}'"
  exit 2
fi

shift
for arg in "$@"; do
  if [[ "$arg" == "--autofix" ]]; then
    enable_autofix="true"
  elif [[ "$scenario_pattern" == "$default_scenario_pattern" ]]; then
    scenario_pattern="$arg"
  else
    echo "ERROR: unsupported argument '$arg'"
    echo "Usage: bash bubbles/scripts/artifact-lint.sh specs/<NNN-feature-name> [scenario-regex] [--autofix]"
    exit 2
  fi
done

if [[ ! -d "$feature_dir" ]]; then
  echo "ERROR: feature directory not found: $feature_dir"
  exit 2
fi

if [[ "$enable_autofix" == "true" ]]; then
  autofix_script="$SCRIPT_DIR/report-section-autofix.sh"
  if [[ ! -x "$autofix_script" ]]; then
    echo "ERROR: autofix requested but helper is missing or not executable: $autofix_script"
    exit 2
  fi

  echo "🔧 Running report section autofix before lint"
  bash "$autofix_script" "$feature_dir" --write
fi

failures=0
dod_total_checkboxes=0
dod_unchecked_count=0
dod_unchecked_items=""

fail() {
  local message="$1"
  echo "❌ $message"
  fun_fail
  failures=$((failures + 1))
}

warn() {
  local message="$1"
  echo "⚠️  $message"
  fun_warn
}

pass() {
  local message="$1"
  echo "✅ $message"
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

json_first_number() {
  local key="$1"
  local file="$2"
  if [[ ! -f "$file" ]]; then
    return 0
  fi

  grep -Eo '"'"$key"'"[[:space:]]*:[[:space:]]*[0-9]+' "$file" \
    | head -n 1 \
    | sed -E 's/.*"'"$key"'"[[:space:]]*:[[:space:]]*([0-9]+)/\1/'
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

extract_array_block() {
  local array_key="$1"
  local file="$2"
  awk '/"'"$array_key"'"[[:space:]]*:/ {capture=1} capture {print} capture && /\]/ {exit}' "$file"
}

extract_nested_array_block() {
  local parent_key="$1"
  local array_key="$2"
  local file="$3"
  grep -A60 '"'"$parent_key"'"' "$file" 2>/dev/null \
    | awk '/"'"$array_key"'"[[:space:]]*:/ {capture=1} capture {print} capture && /\]/ {exit}'
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

cleanup_tmp_artifacts() {
  if [[ -n "$combined_scopes_tmp" ]] && [[ -f "$combined_scopes_tmp" ]]; then
    rm -f "$combined_scopes_tmp"
  fi
}

trap cleanup_tmp_artifacts EXIT

scope_layout="$(detect_scope_layout)"
scope_index_file="$feature_dir/scopes/_index.md"
scope_files=()
report_files=()

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
report_file=""
if [[ ${#report_files[@]} -gt 0 ]]; then
  report_file="${report_files[0]}"
fi

relative_artifact_path() {
  local artifact_path="$1"
  echo "${artifact_path#$feature_dir/}"
}

required_files=(
  "spec.md"
  "design.md"
  "uservalidation.md"
  "state.json"
)

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

# Forbidden sidecar artifacts (see artifact-ownership.md → Forbidden Artifacts)
# These filenames fragment ownership and bypass validation gates. Their content
# belongs inside spec.md / design.md / scopes.md / report.md.
#
# NOTE: tasks.md, data-model.md, requirements.md (under checklists/), and
# test-plan.md are NOT forbidden — they are produced by the speckit workflow
# (which coexists with bubbles) and serve different purposes.
forbidden_artifacts=(
  "ux.md"
  "wireframes.md"
  "flows.md"
  "user-flows.md"
  "screens.md"
  "actors.md"
  "scenarios.md"
  "use-cases.md"
  "architecture.md"
  "tech-design.md"
  "dod.md"
  "gherkin.md"
  "evidence.md"
  "results.md"
)

forbidden_found=0
while IFS= read -r forbidden_path; do
  [[ -z "$forbidden_path" ]] && continue
  forbidden_found=$((forbidden_found + 1))
  rel_path="${forbidden_path#$feature_dir/}"
  fail "Forbidden sidecar artifact present: $rel_path — content MUST live inside spec.md / design.md / scopes.md / report.md (see artifact-ownership.md → Forbidden Artifacts)"
done < <(
  for forbidden_name in "${forbidden_artifacts[@]}"; do
    find "$feature_dir" -type f -name "$forbidden_name" 2>/dev/null
  done
)

if [[ "$forbidden_found" -eq 0 ]]; then
  pass "No forbidden sidecar artifacts present"
fi

for scope_path in "${scope_files[@]}"; do
  [[ -f "$scope_path" ]] || continue

  scope_label="${scope_path#$feature_dir/}"
  if grep -Eq '^### Definition of Done' "$scope_path"; then
    pass "Found DoD section in $scope_label"

    dod_section_content="$({
      awk '
        /^### Definition of Done/ {in_dod=1; next}
        /^### / {if (in_dod) exit}
        in_dod {print}
      ' "$scope_path"
    } || true)"

    local_dod_total="$({ echo "$dod_section_content" | grep -Ec '^- \[( |x)\] '; } || true)"
    local_unchecked_items="$({ echo "$dod_section_content" | grep -E '^- \[ \] '; } || true)"
    local_unchecked_count="$({ echo "$dod_section_content" | grep -Ec '^- \[ \] '; } || true)"
    dod_total_checkboxes=$((dod_total_checkboxes + local_dod_total))
    dod_unchecked_count=$((dod_unchecked_count + local_unchecked_count))

    if [[ "$local_dod_total" -gt 0 ]]; then
      pass "$scope_label DoD contains checkbox items"
    else
      fail "$scope_label DoD section has no checkbox items (- [ ] / - [x])"
    fi

    if [[ -n "$local_unchecked_items" ]]; then
      while IFS= read -r unchecked_item; do
        [[ -n "$unchecked_item" ]] || continue
        dod_unchecked_items+="$scope_label: $unchecked_item"$'\n'
      done <<< "$local_unchecked_items"
    fi

    dod_bullet_lines="$({ echo "$dod_section_content" | grep -E '^- '; } || true)"
    invalid_dod_bullets="$({ echo "$dod_bullet_lines" | grep -Ev '^- \[( |x)\] '; } || true)"
    if [[ -n "$invalid_dod_bullets" ]]; then
      fail "$scope_label DoD contains non-checkbox bullet items"
      echo "$invalid_dod_bullets" | sed 's/^/   -> /'
    else
      pass "All DoD bullet items use checkbox syntax in $scope_label"
    fi
  else
    fail "$scope_label is missing '### Definition of Done' section"
  fi
done

uservalidation_file="$feature_dir/uservalidation.md"
if [[ -f "$uservalidation_file" ]]; then
  if grep -Eq '^## Checklist' "$uservalidation_file"; then
    pass "Found Checklist section in uservalidation.md"

    checklist_section_content="$({
      awk '
        /^## Checklist/ {in_checklist=1; next}
        /^## / {if (in_checklist) exit}
        in_checklist {print}
      ' "$uservalidation_file"
    } || true)"

    checklist_checkbox_lines="$({ echo "$checklist_section_content" | grep -E '^- \[(x| )\] '; } || true)"
    if [[ -z "$checklist_checkbox_lines" ]]; then
      fail "uservalidation checklist has no checkbox entries"
    else
      pass "uservalidation checklist contains checkbox entries"
    fi

    baseline_checked_lines="$({ echo "$checklist_section_content" | grep -E '^- \[x\] '; } || true)"
    if [[ -z "$baseline_checked_lines" ]]; then
      fail "uservalidation checklist has no checked-by-default [x] entry"
    else
      pass "uservalidation checklist has checked-by-default entries"
    fi

    checklist_bullet_lines="$({ echo "$checklist_section_content" | grep -E '^- '; } || true)"
    invalid_checklist_bullets="$({ echo "$checklist_bullet_lines" | grep -Ev '^- \[(x| )\] '; } || true)"
    if [[ -n "$invalid_checklist_bullets" ]]; then
      fail "uservalidation checklist contains non-checkbox bullet items"
      echo "$invalid_checklist_bullets" | sed 's/^/   -> /'
    else
      pass "All checklist bullet items use checkbox syntax"
    fi
  else
    legacy_checklist_section="$({
      awk '
        /^# / {next}
        {print}
      ' "$uservalidation_file"
    } || true)"
    legacy_checkbox_lines="$({ echo "$legacy_checklist_section" | grep -E '^- \[(x| )\] '; } || true)"
    if [[ -n "$legacy_checkbox_lines" ]]; then
      warn "uservalidation.md is using legacy checklist layout without '## Checklist' section"
    else
      fail "uservalidation.md is missing '## Checklist' section"
    fi
  fi
fi

state_file="$feature_dir/state.json"
state_version=""
state_status=""
state_certification_status=""
state_workflow_mode=""
state_completed_phases_block=""
state_execution_phase_claims_block=""
state_certified_completed_phases_block=""
state_completed_scopes_block=""
wi_parity_present="false"
wi_canonical_count=""
wi_provisional_count=""
wi_post_migration_target=""
wi_migration_status=""
wi_migration_source=""
wi_trace_matrix=""
if [[ -f "$state_file" ]]; then
  state_version="$(json_first_number "version" "$state_file" || true)"
  state_status="$({
    grep -Eo '"status"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" \
      | head -n 1 \
      | sed -E 's/.*"status"[[:space:]]*:[[:space:]]*"([^"]+)"/\1/'
  } || true)"

  state_certification_status="$(json_nested_string "certification" "status" "$state_file" || true)"

  state_workflow_mode="$({
    grep -Eo '"workflowMode"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" \
      | head -n 1 \
      | sed -E 's/.*"workflowMode"[[:space:]]*:[[:space:]]*"([^"]+)"/\1/'
  } || true)"

  state_execution_phase_claims_block="$(extract_nested_array_block "execution" "completedPhaseClaims" "$state_file" || true)"
  state_certified_completed_phases_block="$(extract_nested_array_block "certification" "certifiedCompletedPhases" "$state_file" || true)"
  state_completed_scopes_block="$(extract_nested_array_block "certification" "completedScopes" "$state_file" || true)"

  if [[ -z "$state_completed_scopes_block" ]]; then
    state_completed_scopes_block="$(extract_array_block "completedScopes" "$state_file" || true)"
  fi

  if [[ -n "$state_certified_completed_phases_block" ]]; then
    state_completed_phases_block="$state_certified_completed_phases_block"
  elif [[ -n "$state_execution_phase_claims_block" ]]; then
    state_completed_phases_block="$state_execution_phase_claims_block"
  else
    state_completed_phases_block="$(extract_array_block "completedPhases" "$state_file" || true)"
  fi

  if [[ -z "$state_status" ]]; then
    fail "Unable to determine state.json status field"
  else
    pass "Detected state.json status: $state_status"

    if [[ "$state_status" == "done" ]]; then
      if [[ "$dod_total_checkboxes" -eq 0 ]]; then
        fail "state.json status 'done' requires at least one DoD checkbox item in the resolved scope artifacts"
      elif [[ "$dod_unchecked_count" -gt 0 ]]; then
        fail "state.json status 'done' is invalid: DoD contains unchecked items"
        echo "$dod_unchecked_items" | sed 's/^/   -> /'
      else
        pass "DoD completion gate passed for status 'done' (all DoD checkboxes are checked)"
      fi
    fi
  fi

  if [[ -z "$state_workflow_mode" ]]; then
    pass "No workflowMode found in state.json (mode-specific report gates skipped)"
  else
    pass "Detected state.json workflowMode: $state_workflow_mode"

    # ============================================================
    # STATE.JSON SCHEMA VALIDATION (legacy v2 + v3 control plane)
    # ============================================================
    if [[ -n "$state_version" && "$state_version" -ge 3 ]]; then
      for req_field in "status" "execution" "certification" "policySnapshot"; do
        if grep -q "\"$req_field\"" "$state_file"; then
          pass "state.json v3 has required field: $req_field"
        else
          fail "state.json v3 missing required field: $req_field"
        fi
      done
      for recommended_field in "transitionRequests" "reworkQueue" "executionHistory"; do
        if grep -q "\"$recommended_field\"" "$state_file"; then
          pass "state.json v3 has recommended field: $recommended_field"
        else
          warn "state.json v3 missing recommended field: $recommended_field"
        fi
      done
      if [[ -n "$state_certification_status" ]]; then
        if [[ "$state_certification_status" == "$state_status" ]]; then
          pass "Top-level status matches certification.status"
        else
          fail "Top-level status '$state_status' does not match certification.status '$state_certification_status'"
        fi
      else
        fail "state.json v3 missing certification.status"
      fi
    else
      for req_field in "status" "completedPhases" "completedScopes" "lastUpdatedAt"; do
        if grep -q "\"$req_field\"" "$state_file"; then
          pass "state.json has required field: $req_field"
        else
          warn "state.json missing recommended field: $req_field"
        fi
      done
    fi

    # Check for deprecated field usage
    for dep_field in "featureId" "featureFile" "selectedScopeName" "scopeProgress" "statusDiscipline" "scopeLayout"; do
      if grep -q "\"$dep_field\"" "$state_file"; then
        warn "state.json uses deprecated field '$dep_field' — see scope-workflow.md state.json canonical schema v2"
      fi
    done

    # Check completedScopes is array of strings (not numbers)
    if [[ -n "$state_completed_scopes_block" ]]; then
      if echo "$state_completed_scopes_block" | grep -Eq '\[[[:space:]]*[0-9]'; then
        warn "state.json completedScopes contains numbers — canonical schema requires string scope identifiers"
      fi
    fi

    # Check completed phases / claims are array of strings (not objects)
    if [[ -n "$state_completed_phases_block" ]]; then
      if echo "$state_completed_phases_block" | grep -q '"completedAt"'; then
        warn "state.json completion phase block uses legacy object format — supported for compatibility; prefer simple string entries in new state files"
      fi
    fi

    if [[ "$state_workflow_mode" == "full-delivery" ]] && [[ "$state_status" == "done" ]]; then
      strict_required_phases=("validate" "audit" "chaos")
      for strict_phase in "${strict_required_phases[@]}"; do
        if echo "$state_completed_phases_block" | grep -Eq "\"$strict_phase\""; then
          pass "Strict mode completedPhases includes '$strict_phase'"
        else
          fail "$state_workflow_mode done status requires completedPhases to include '$strict_phase'"
        fi
      done
    fi

    # ============================================================
    # STATUS CEILING ENFORCEMENT (Anti-Fabrication)
    # ============================================================
    if [[ "$state_status" == "done" ]]; then
      case "$state_workflow_mode" in
          fix|full-delivery|value-first-e2e-batch|feature-bootstrap|bugfix-fastlane|chaos-hardening|harden-to-doc|gaps-to-doc|harden-gaps-to-doc|reconcile-to-doc|test-to-doc|chaos-to-doc|batch-implement|batch-harden|batch-gaps|batch-harden-gaps|batch-improve-existing|batch-reconcile-to-doc|product-to-delivery|improve-existing)
          pass "Workflow mode '$state_workflow_mode' allows status 'done'"
          ;;
        spec-scope-hardening)
          fail "Workflow mode 'spec-scope-hardening' ceiling is 'specs_hardened', NOT 'done' — FABRICATION"
          ;;
        docs-only)
          fail "Workflow mode 'docs-only' ceiling is 'docs_updated', NOT 'done' — FABRICATION"
          ;;
        validate-only|audit-only|validate-to-doc)
          fail "Workflow mode '$state_workflow_mode' ceiling is 'validated', NOT 'done' — FABRICATION"
          ;;
        resume-only)
          fail "Workflow mode 'resume-only' ceiling is 'in_progress', NOT 'done' — FABRICATION"
          ;;
        *)
          fail "Unknown workflow mode '$state_workflow_mode' with status 'done' — cannot verify ceiling"
          ;;
      esac
    fi

    # ============================================================
    # SCOPE STATUS CROSS-REFERENCE (Anti-Fabrication)
    # ============================================================
    if [[ "$state_status" == "done" ]] && [[ -f "$scopes_file" ]]; then
      not_started_scopes="$({ grep -cE '\*\*Status:\*\*.*Not Started' "$scopes_file"; } || true)"
      in_progress_scopes="$({ grep -cE '\*\*Status:\*\*.*In Progress' "$scopes_file"; } || true)"
      done_scopes="$({ grep -cE '\*\*Status:\*\*.*Done' "$scopes_file"; } || true)"
      total_scopes=$((not_started_scopes + in_progress_scopes + done_scopes))

      if [[ "$not_started_scopes" -gt 0 ]]; then
        fail "state.json says 'done' but scopes.md has $not_started_scopes scope(s) still 'Not Started' — FABRICATION"
      fi
      if [[ "$in_progress_scopes" -gt 0 ]]; then
        fail "state.json says 'done' but scopes.md has $in_progress_scopes scope(s) still 'In Progress' — FABRICATION"
      fi
      if [[ "$total_scopes" -eq 0 ]]; then
        fail "state.json says 'done' but scopes.md has no scope status markers — FABRICATION"
      fi
      if [[ "$not_started_scopes" -eq 0 ]] && [[ "$in_progress_scopes" -eq 0 ]] && [[ "$total_scopes" -gt 0 ]]; then
        pass "All $done_scopes scope(s) in scopes.md are marked Done"
      fi

      # Cross-check completedScopes array in state.json
      state_completed_scopes="$state_completed_scopes_block"
      state_completed_scopes_empty="$({ echo "$state_completed_scopes" | grep -cE '^\s*\[\s*\]\s*$|"completedScopes"[[:space:]]*:[[:space:]]*\[\s*\]'; } || true)"

      if [[ "$done_scopes" -gt 0 ]] && echo "$state_completed_scopes" | grep -qE '\[\s*\]'; then
        fail "scopes.md shows $done_scopes Done scope(s) but state.json completedScopes is empty — INCONSISTENCY"
      fi
    fi

    # ============================================================
    # SPECIALIST PHASE COMPLETION FOR ALL DONE MODES (Anti-Fabrication)
    # ============================================================
    if [[ "$state_status" == "done" ]]; then
      mode_required_specialists=()
      case "$state_workflow_mode" in
        full-delivery|value-first-e2e-batch)
          mode_required_specialists=("implement" "test" "docs" "validate" "audit" "chaos")
          ;;
        full-delivery)
          mode_required_specialists=("implement" "test" "regression" "simplify" "gaps" "harden" "stabilize" "security" "docs" "validate" "audit" "chaos")
          ;;
        feature-bootstrap)
          mode_required_specialists=("implement" "test" "docs" "validate" "audit")
          ;;
        bugfix-fastlane)
          mode_required_specialists=("implement" "test" "validate" "audit")
          ;;
        chaos-hardening)
          mode_required_specialists=("chaos" "implement" "test" "validate" "audit")
          ;;
        harden-to-doc)
          mode_required_specialists=("harden" "test" "chaos" "validate" "audit" "docs")
          ;;
        gaps-to-doc)
          mode_required_specialists=("gaps" "test" "chaos" "validate" "audit" "docs")
          ;;
        test-to-doc)
          mode_required_specialists=("test" "validate" "audit" "docs")
          ;;
        chaos-to-doc)
          mode_required_specialists=("chaos" "validate" "audit" "docs")
          ;;
        validate-to-doc)
          mode_required_specialists=("validate" "audit" "docs")
          ;;
      esac

      if [[ ${#mode_required_specialists[@]} -gt 0 ]]; then
        missing_specialist_count=0
        for sp in "${mode_required_specialists[@]}"; do
          if echo "$state_completed_phases_block" | grep -qE "\"$sp\""; then
            pass "Required specialist phase '$sp' found in execution/certification phase records"
          else
            fail "Required specialist phase '$sp' missing from execution/certification phase records (Gate G022 — FABRICATION)"
            missing_specialist_count=$((missing_specialist_count + 1))
          fi
        done
        if [[ "$missing_specialist_count" -gt 0 ]]; then
          fail "$missing_specialist_count of ${#mode_required_specialists[@]} required specialist phases are MISSING"
        fi
      fi
    fi

    # ============================================================
    # SPEC REVIEW ENFORCEMENT FOR LEGACY-IMPROVEMENT MODES
    # ============================================================
    if [[ "$state_status" == "done" ]]; then
      spec_review_required_modes="improve-existing|reconcile-to-doc|redesign-existing|full-delivery"
      if echo "$state_workflow_mode" | grep -qE "^($spec_review_required_modes)$"; then
        if echo "$state_completed_phases_block" | grep -qE '"spec-review"'; then
          pass "Spec-review phase recorded for legacy-improvement mode '$state_workflow_mode'"
        else
          fail "Legacy-improvement mode '$state_workflow_mode' requires spec-review phase but 'spec-review' is NOT in completed phases"
        fi
      fi
    fi

    # ============================================================
    # TIMESTAMP PLAUSIBILITY CHECK (Anti-Fabrication)
    # ============================================================
    if [[ "$state_status" == "done" ]]; then
      phase_timestamps=()
      while IFS= read -r ts; do
        phase_timestamps+=("$ts")
      done < <(grep -oE '"completedAt"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" 2>/dev/null \
        | sed -E 's/.*"completedAt"[[:space:]]*:[[:space:]]*"([^"]+)"/\1/' || true)

      if [[ ${#phase_timestamps[@]} -ge 3 ]]; then
        prev_epoch=0
        intervals=()
        all_parseable="true"
        for ts in "${phase_timestamps[@]}"; do
          epoch="$(date -d "$ts" +%s 2>/dev/null || true)"
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
          all_identical="true"
          first_interval="${intervals[0]}"
          for interval in "${intervals[@]}"; do
            if [[ "$interval" -ne "$first_interval" ]]; then
              all_identical="false"
              break
            fi
          done
          if [[ "$all_identical" == "true" ]]; then
            fail "Completion timestamps are uniformly spaced (${first_interval}s apart) — FABRICATION INDICATOR"
          else
            pass "Phase timestamps have variable intervals (plausible)"
          fi

          # Check if all timestamps span less than 5 seconds (impossible for multi-phase work)
          min_epoch="$(date -d "${phase_timestamps[0]}" +%s 2>/dev/null || true)"
          max_epoch="$min_epoch"
          for ts in "${phase_timestamps[@]}"; do
            epoch="$(date -d "$ts" +%s 2>/dev/null || true)"
            [[ -n "$epoch" ]] || continue
            [[ "$epoch" -lt "$min_epoch" ]] && min_epoch="$epoch"
            [[ "$epoch" -gt "$max_epoch" ]] && max_epoch="$epoch"
          done
          spread=$((max_epoch - min_epoch))
          if [[ "$spread" -lt 5 ]] && [[ ${#phase_timestamps[@]} -ge 3 ]]; then
            fail "All ${#phase_timestamps[@]} phase timestamps span only ${spread}s — impossible for real execution"
          fi
        fi
      fi
    fi

    # ============================================================
    # PHASE-SCOPE COHERENCE CHECK (Gate G027 — Anti-Fabrication)
    # ============================================================
    if [[ "$state_status" == "done" ]]; then
      # Check if implementation phases are claimed
      has_implement_phase="false"
      has_test_phase="false"
      if echo "$state_completed_phases_block" | grep -qE '"implement"'; then
        has_implement_phase="true"
      fi
      if echo "$state_completed_phases_block" | grep -qE '"test"'; then
        has_test_phase="true"
      fi

      if [[ "$has_implement_phase" == "true" || "$has_test_phase" == "true" ]]; then
        # Extract completedScopes count
        lint_completed_scopes_count="$({
          echo "$state_completed_scopes_block" \
          | sed -E '1s/.*"completedScopes"[[:space:]]*:[[:space:]]*\[//' \
          | grep -cE '"[^"]+"' || true
        } || true)"

        if [[ "$lint_completed_scopes_count" -eq 0 ]]; then
          fail "Execution/certified phases claim implement/test but completedScopes is EMPTY — PHASE-SCOPE INCOHERENCE (Gate G027)"
        fi

        if [[ "$done_scopes" -eq 0 ]]; then
          fail "Execution/certified phases claim implement/test but ZERO scopes are marked Done — FABRICATION (Gate G027)"
        fi

        # Check for full pipeline claim with partial scope completion
        claimed_lifecycle_count="$(echo "$state_completed_phases_block" | grep -cE '"(implement|test|docs|validate|audit|chaos)"' || true)"
        if [[ "$claimed_lifecycle_count" -ge 5 ]] && [[ "$done_scopes" -lt "$total_scopes" ]] && [[ "$total_scopes" -gt 0 ]]; then
          fail "Execution/certified phases claim $claimed_lifecycle_count lifecycle phases but only $done_scopes of $total_scopes scopes are Done — INCOHERENCE (Gate G027)"
        fi

        if [[ "$lint_completed_scopes_count" -gt 0 ]] && [[ "$done_scopes" -gt 0 ]]; then
          pass "Phase-scope coherence verified (Gate G027)"
        fi
      fi
    fi
  fi

  # ============================================================
  # WI PARITY (Canonical + Provisional Intake) VALIDATION
  # ============================================================
  wi_canonical_count="$({
    grep -Eo '"canonicalCount"[[:space:]]*:[[:space:]]*[0-9]+' "$state_file" \
      | head -n 1 \
      | sed -E 's/.*:[[:space:]]*([0-9]+)/\1/'
  } || true)"

  wi_provisional_count="$({
    grep -Eo '"provisionalIntakeCount"[[:space:]]*:[[:space:]]*[0-9]+' "$state_file" \
      | head -n 1 \
      | sed -E 's/.*:[[:space:]]*([0-9]+)/\1/'
  } || true)"

  wi_post_migration_target="$({
    grep -Eo '"postMigrationTargetCount"[[:space:]]*:[[:space:]]*[0-9]+' "$state_file" \
      | head -n 1 \
      | sed -E 's/.*:[[:space:]]*([0-9]+)/\1/'
  } || true)"

  wi_migration_status="$({
    grep -Eo '"migrationStatus"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" \
      | head -n 1 \
      | sed -E 's/.*"migrationStatus"[[:space:]]*:[[:space:]]*"([^"]+)"/\1/'
  } || true)"

  wi_migration_source="$({
    grep -Eo '"migrationSource"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" \
      | head -n 1 \
      | sed -E 's/.*"migrationSource"[[:space:]]*:[[:space:]]*"([^"]+)"/\1/'
  } || true)"

  wi_trace_matrix="$({
    grep -Eo '"traceMatrix"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" \
      | head -n 1 \
      | sed -E 's/.*"traceMatrix"[[:space:]]*:[[:space:]]*"([^"]+)"/\1/'
  } || true)"

  if [[ -n "$wi_canonical_count$wi_provisional_count$wi_post_migration_target$wi_migration_status" ]]; then
    wi_parity_present="true"
    pass "Detected wiParity metadata in state.json"

    if [[ -z "$wi_canonical_count" ]] || [[ -z "$wi_provisional_count" ]] || [[ -z "$wi_post_migration_target" ]] || [[ -z "$wi_migration_status" ]]; then
      fail "wiParity metadata is incomplete (requires canonicalCount, provisionalIntakeCount, postMigrationTargetCount, migrationStatus)"
    else
      expected_wi_total=$((wi_canonical_count + wi_provisional_count))
      if [[ "$expected_wi_total" -eq "$wi_post_migration_target" ]]; then
        pass "wiParity total is consistent: canonical ($wi_canonical_count) + provisional ($wi_provisional_count) = postMigrationTarget ($wi_post_migration_target)"
      else
        fail "wiParity count mismatch: canonical ($wi_canonical_count) + provisional ($wi_provisional_count) != postMigrationTarget ($wi_post_migration_target)"
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
        pass "Dual-count mode active: provisional intake tracked separately until migration activation"
      fi

      if [[ "$wi_migration_status" == "activated" ]] && [[ "$wi_provisional_count" -gt 0 ]]; then
        fail "wiParity migrationStatus is 'activated' but provisionalIntakeCount is $wi_provisional_count (must be 0 after activation)"
      fi
    fi

    if [[ -n "$wi_migration_source" ]]; then
      wi_migration_source_file="${wi_migration_source%%#*}"
      if [[ -f "$feature_dir/$wi_migration_source_file" ]]; then
        pass "wiParity migrationSource file exists: $wi_migration_source_file"
      else
        fail "wiParity migrationSource file not found: $feature_dir/$wi_migration_source_file"
      fi
    fi

    if [[ -n "$wi_trace_matrix" ]]; then
      if [[ -f "$feature_dir/$wi_trace_matrix" ]]; then
        pass "wiParity traceMatrix file exists: $wi_trace_matrix"
      else
        fail "wiParity traceMatrix file not found: $feature_dir/$wi_trace_matrix"
      fi
    fi
  fi

  # ============================================================
  # NOTEBOOK CORPUS DEPENDENCY MAP VALIDATION
  # ============================================================
  notebook_dependency_block="$({
    awk '
      /"notebookCorpusDependencyMap"[[:space:]]*:/ {capture=1}
      capture {print}
      capture && /\]/ {exit}
    ' "$state_file"
  } || true)"

  if [[ -n "$notebook_dependency_block" ]] && echo "$notebook_dependency_block" | grep -q '"notebookCorpusDependencyMap"'; then
    pass "Detected notebookCorpusDependencyMap in state.json"

    notebook_dependency_refs="$({
      echo "$notebook_dependency_block" \
        | grep -Eo '"[^"]+"' \
        | sed 's/^"//; s/"$//' \
        | grep -v '^notebookCorpusDependencyMap$'
    } || true)"

    if [[ -z "$notebook_dependency_refs" ]]; then
      fail "notebookCorpusDependencyMap is declared but contains no dependency references"
    else
      while IFS= read -r notebook_ref; do
        [[ -n "$notebook_ref" ]] || continue

        notebook_ref_file="${notebook_ref%%#*}"
        notebook_ref_anchor=""
        if [[ "$notebook_ref" == *#* ]]; then
          notebook_ref_anchor="${notebook_ref#*#}"
        fi

        if [[ ! -f "$feature_dir/$notebook_ref_file" ]]; then
          fail "notebookCorpusDependencyMap file not found: $feature_dir/$notebook_ref_file"
          continue
        fi
        pass "notebookCorpusDependencyMap file exists: $notebook_ref_file"

        if [[ -z "$notebook_ref_anchor" ]]; then
          fail "notebookCorpusDependencyMap reference is missing anchor fragment (#...): $notebook_ref"
          continue
        fi

        notebook_anchor_match="false"
        notebook_ref_anchor_compact="$({ echo "$notebook_ref_anchor" | tr -cd '[:alnum:]'; } || true)"
        notebook_headings="$({ grep -E '^#{1,6}[[:space:]]+' "$feature_dir/$notebook_ref_file"; } || true)"
        while IFS= read -r notebook_heading_line; do
          [[ -n "$notebook_heading_line" ]] || continue
          notebook_heading_text="$(echo "$notebook_heading_line" | sed -E 's/^#{1,6}[[:space:]]+//')"
          notebook_heading_slug="$({
            echo "$notebook_heading_text" \
              | tr '[:upper:]' '[:lower:]' \
              | sed -E 's/[^a-z0-9 -]+/ /g; s/[[:space:]]+/-/g; s/-+/-/g; s/^-+//; s/-+$//'
          } || true)"
          notebook_heading_slug_compact="$({ echo "$notebook_heading_slug" | tr -cd '[:alnum:]'; } || true)"
          if [[ "$notebook_heading_slug" == "$notebook_ref_anchor" ]] || [[ "$notebook_heading_slug_compact" == "$notebook_ref_anchor_compact" ]]; then
            notebook_anchor_match="true"
            break
          fi
        done <<< "$notebook_headings"

        if [[ "$notebook_anchor_match" == "true" ]]; then
          pass "notebookCorpusDependencyMap anchor resolved: $notebook_ref"
        else
          fail "notebookCorpusDependencyMap anchor not found in $notebook_ref_file: #$notebook_ref_anchor"
        fi
      done <<< "$notebook_dependency_refs"
    fi
  fi

  # ============================================================
  # TRANSCRIPT DEPENDENCY MAP VALIDATION
  # ============================================================
  transcript_dependency_block="$({
    awk '
      /"transcriptDependencyMap"[[:space:]]*:/ {capture=1}
      capture {print}
      capture && /\]/ {exit}
    ' "$state_file"
  } || true)"

  if [[ -n "$transcript_dependency_block" ]] && echo "$transcript_dependency_block" | grep -q '"transcriptDependencyMap"'; then
    pass "Detected transcriptDependencyMap in state.json"

    transcript_dependency_refs="$({
      echo "$transcript_dependency_block" \
        | grep -Eo '"[^"]+"' \
        | sed 's/^"//; s/"$//' \
        | grep -v '^transcriptDependencyMap$'
    } || true)"

    if [[ -z "$transcript_dependency_refs" ]]; then
      fail "transcriptDependencyMap is declared but contains no dependency references"
    else
      while IFS= read -r transcript_ref; do
        [[ -n "$transcript_ref" ]] || continue

        transcript_ref_file="${transcript_ref%%#*}"
        transcript_ref_anchor=""
        if [[ "$transcript_ref" == *#* ]]; then
          transcript_ref_anchor="${transcript_ref#*#}"
        fi

        if [[ ! -f "$feature_dir/$transcript_ref_file" ]]; then
          fail "transcriptDependencyMap file not found: $feature_dir/$transcript_ref_file"
          continue
        fi
        pass "transcriptDependencyMap file exists: $transcript_ref_file"

        if [[ -z "$transcript_ref_anchor" ]]; then
          fail "transcriptDependencyMap reference is missing anchor fragment (#...): $transcript_ref"
          continue
        fi

        transcript_anchor_match="false"
        transcript_ref_anchor_compact="$({ echo "$transcript_ref_anchor" | tr -cd '[:alnum:]'; } || true)"
        transcript_headings="$({ grep -E '^#{1,6}[[:space:]]+' "$feature_dir/$transcript_ref_file"; } || true)"
        while IFS= read -r transcript_heading_line; do
          [[ -n "$transcript_heading_line" ]] || continue
          transcript_heading_text="$(echo "$transcript_heading_line" | sed -E 's/^#{1,6}[[:space:]]+//')"
          transcript_heading_slug="$({
            echo "$transcript_heading_text" \
              | tr '[:upper:]' '[:lower:]' \
              | sed -E 's/[^a-z0-9 -]+/ /g; s/[[:space:]]+/-/g; s/-+/-/g; s/^-+//; s/-+$//'
          } || true)"
          transcript_heading_slug_compact="$({ echo "$transcript_heading_slug" | tr -cd '[:alnum:]'; } || true)"
          if [[ "$transcript_heading_slug" == "$transcript_ref_anchor" ]] || [[ "$transcript_heading_slug_compact" == "$transcript_ref_anchor_compact" ]]; then
            transcript_anchor_match="true"
            break
          fi
        done <<< "$transcript_headings"

        if [[ "$transcript_anchor_match" == "true" ]]; then
          pass "transcriptDependencyMap anchor resolved: $transcript_ref"
        else
          fail "transcriptDependencyMap anchor not found in $transcript_ref_file: #$transcript_ref_anchor"
        fi
      done <<< "$transcript_dependency_refs"
    fi
  fi
fi

for report_file in "${report_files[@]}"; do
if [[ -f "$report_file" ]]; then
  required_report_headers=(
    '^###[[:space:]]+Summary|^##[[:space:]]+Summary'
    '^###[[:space:]]+Completion Statement|^##[[:space:]]+Completion Statement'
    '^###[[:space:]]+Test Evidence|^##[[:space:]]+Test Evidence'
  )

  for required_header in "${required_report_headers[@]}"; do
    if grep -Eq "$required_header" "$report_file"; then
      pass "report.md contains section matching: ${required_header#^}"
    else
      fail "report.md missing required section matching: ${required_header#^}"
    fi
  done

  pending_placeholders="$({ grep -nE '\[PENDING[^]]*\]|header only initially|Ready for /bubbles\.|Re-run /bubbles\.validate|Commit the fix|Record DoD evidence|Run full E2E suite|^#{1,4}[[:space:]]+Next Steps|^-[[:space:]]+Next Steps|Recommended routing:|Recommended resolution:|Recommended next move' "$report_file"; } || true)"
  if [[ -n "$pending_placeholders" ]]; then
    fail "report.md contains unresolved placeholder or manual follow-up language"
    echo "$pending_placeholders" | sed 's/^/   -> /'
  fi

  should_enforce_mode_gates="false"
  case "$state_status" in
    done|validated|docs_updated|specs_hardened)
      should_enforce_mode_gates="true"
      ;;
  esac

  if [[ "$should_enforce_mode_gates" == "true" ]] && [[ -z "$state_workflow_mode" ]]; then
    fail "state.json status '$state_status' requires workflowMode to enforce mode-specific report gates"
  fi

  mode_required_evidence_headers=()
  if [[ "$should_enforce_mode_gates" == "true" ]] && [[ -n "$state_workflow_mode" ]]; then
    case "$state_workflow_mode" in
      full-delivery|value-first-e2e-batch|chaos-hardening|harden-to-doc|gaps-to-doc|harden-gaps-to-doc|reconcile-to-doc|chaos-to-doc|batch-implement|batch-harden|batch-gaps|batch-harden-gaps|batch-improve-existing|batch-reconcile-to-doc|product-to-delivery|improve-existing)
        mode_required_evidence_headers=(
          '^### Validation Evidence'
          '^### Audit Evidence'
          '^### Chaos Evidence'
        )
        ;;
      feature-bootstrap|bugfix-fastlane|test-to-doc)
        mode_required_evidence_headers=(
          '^### Validation Evidence'
          '^### Audit Evidence'
        )
        ;;
      validate-only|validate-to-doc)
        mode_required_evidence_headers=(
          '^### Validation Evidence'
          '^### Audit Evidence'
        )
        ;;
      audit-only)
        mode_required_evidence_headers=(
          '^### Audit Evidence'
        )
        ;;
    esac
  fi

  if [[ ${#mode_required_evidence_headers[@]} -gt 0 ]]; then
    for required_mode_header in "${mode_required_evidence_headers[@]}"; do
      if grep -Eq "$required_mode_header" "$report_file"; then
        pass "workflowMode gate satisfied: ${required_mode_header#^}"
      else
        fail "state.json workflowMode '$state_workflow_mode' requires report.md section: ${required_mode_header#^}"
      fi
    done
  elif [[ "$should_enforce_mode_gates" == "true" ]] && [[ -n "$state_workflow_mode" ]]; then
    pass "No mode-specific report gates configured for workflowMode '$state_workflow_mode'"
  elif [[ "$should_enforce_mode_gates" == "true" ]] && [[ -z "$state_workflow_mode" ]]; then
    pass "Mode-specific report gates not evaluated because workflowMode is missing"
  else
    pass "Mode-specific report gates skipped (status not in promotion set)"
  fi

  if [[ "$state_workflow_mode" == "full-delivery" ]] && [[ "$state_status" == "done" ]]; then
    strict_sections=("Validation Evidence|bubbles.validate" "Audit Evidence|bubbles.audit" "Chaos Evidence|bubbles.chaos")
    for strict_entry in "${strict_sections[@]}"; do
      strict_section="${strict_entry%%|*}"
      strict_agent="${strict_entry##*|}"

      strict_section_content="$({
        awk -v section="$strict_section" '
          $0 ~ "^### " section "$" {capture=1; next}
          capture && /^### / {exit}
          capture {print}
        ' "$report_file"
      } || true)"

      if [[ -z "$strict_section_content" ]]; then
        fail "$state_workflow_mode done status requires populated section: ### $strict_section"
        continue
      fi

      if echo "$strict_section_content" | grep -Eq '^\*\*Executed:\*\* YES'; then
        pass "Strict section '$strict_section' includes Executed: YES"
      else
        fail "$state_workflow_mode done status requires '**Executed:** YES' in section '$strict_section'"
      fi

      if echo "$strict_section_content" | grep -Eq '^\*\*Command:\*\* '; then
        pass "Strict section '$strict_section' includes command evidence"
      else
        fail "$state_workflow_mode done status requires '**Command:**' evidence in section '$strict_section'"
      fi

      if echo "$strict_section_content" | grep -Eq "^\*\*Phase Agent:\*\* .*${strict_agent}"; then
        pass "Strict section '$strict_section' includes phase agent marker '${strict_agent}'"
      else
        fail "$state_workflow_mode done status requires '**Phase Agent:** ${strict_agent}' marker in section '$strict_section'"
      fi
    done
  fi

  value_selection_lint_script="$SCRIPT_DIR/value-selection-lint.sh"
  if [[ -x "$value_selection_lint_script" ]]; then
    if grep -Eq 'value-first-e2e-batch|Value-First Selection Cycle' "$report_file"; then
      if value_selection_output="$(bash "$value_selection_lint_script" "$report_file" 2>&1)"; then
        pass "Value-first selection rationale lint passed for report.md"
      else
        fail "Value-first selection rationale lint failed for report.md"
        while IFS= read -r line; do
          echo "   -> $line"
        done <<< "$value_selection_output"
      fi
    else
      pass "Value-first selection rationale lint skipped (not a value-first report)"
    fi
  else
    fail "Missing executable lint helper: $value_selection_lint_script"
  fi

  if grep -Eq "^#### ${scenario_pattern}" "$report_file"; then
    if scn_037_path_output="$(awk -v pattern="$scenario_pattern" '
      BEGIN {
        scenario_re = "^#### (" pattern ")$"
      }

      function check_current() {
        if (current != "") {
          missing_list = ""
          if (!has_happy) missing_list = missing_list " Happy"
          if (!has_error) missing_list = missing_list " Error"
          if (!has_degraded) missing_list = missing_list " Degraded"
          if (missing_list != "") {
            print current " missing:" missing_list
            missing = 1
          }
        }
      }

      $0 ~ scenario_re {
        check_current()
        current = $2
        has_happy = 0
        has_error = 0
        has_degraded = 0
        next
      }

      /^#### / {
        check_current()
        current = ""
        next
      }

      {
        if (current != "") {
          if ($0 ~ /^- Happy Path:/) has_happy = 1
          if ($0 ~ /^- Error Path:/) has_error = 1
          if ($0 ~ /^- Degraded Path:/) has_degraded = 1
        }
      }

      END {
        check_current()
        if (missing) exit 1
      }
    ' "$report_file" 2>&1)"; then
      pass "Scenario sections matching pattern include Happy/Error/Degraded path placeholders"
    else
      fail "Scenario sections matching pattern are missing required Happy/Error/Degraded path placeholders"
      while IFS= read -r line; do
        [[ -n "$line" ]] && echo "   -> $line"
      done <<< "$scn_037_path_output"
    fi
  else
    pass "Scenario path-placeholder lint skipped (no matching scenario sections found)"
  fi
fi

done

# ============================================================
# Anti-Fabrication Evidence Checks (Gates G020, G021)
# ============================================================

echo ""
echo "=== Anti-Fabrication Evidence Checks ==="

# Check 1: DoD evidence presence — each [x] item must have evidence block below it
for scope_path in "${scope_files[@]}"; do
  [[ -f "$scope_path" ]] || continue
  checked_items_without_evidence=0
  while IFS= read -r line; do
    if echo "$line" | grep -qiE '(→[[:space:]]*Evidence:|Evidence:)'; then
      continue
    fi

    item_line_num="$({ grep -nF -- "$line" "$scope_path" | head -1 | cut -d: -f1; } || true)"
    if [[ -n "$item_line_num" ]]; then
      next_lines="$({ sed -n "$((item_line_num+1)),$((item_line_num+15))p" "$scope_path"; } || true)"
      if echo "$next_lines" | grep -qiE '(Executed:|Command:|Evidence|```|Exit Code:)'; then
        :
      else
        checked_items_without_evidence=$((checked_items_without_evidence + 1))
        fail "DoD item marked [x] has no evidence block in $(relative_artifact_path "$scope_path"): $(echo "$line" | head -c 80)"
      fi
    fi
  done < <(grep -E '^- \[x\] ' "$scope_path" 2>/dev/null || true)

  if [[ "$checked_items_without_evidence" -eq 0 ]]; then
    pass "All checked DoD items in $(relative_artifact_path "$scope_path") have evidence blocks"
  fi
done

# Check 2: Evidence template detection — detect unfilled template placeholders
for scope_path in "${scope_files[@]}"; do
  [[ -f "$scope_path" ]] || continue
  template_placeholders="$({ grep -nE '\[ACTUAL terminal output|\[PASTE VERBATIM terminal output|\[PASTE VERBATIM.*output here' "$scope_path"; } || true)"
  if [[ -n "$template_placeholders" ]]; then
    fail "$(relative_artifact_path "$scope_path") contains unfilled evidence template placeholders (fabrication detected)"
    echo "$template_placeholders" | head -5 | sed 's/^/   -> /'
  else
    pass "No unfilled evidence template placeholders in $(relative_artifact_path "$scope_path")"
  fi
done

for current_report_file in "${report_files[@]}"; do
  [[ -f "$current_report_file" ]] || continue
  report_template_placeholders="$({ grep -nE '\[ACTUAL terminal output|\[exact cmd\]|\[actual exit code\]|\[ACTUAL output|\[PASTE VERBATIM terminal output|\[PASTE VERBATIM.*output here' "$current_report_file"; } || true)"
  if [[ -n "$report_template_placeholders" ]]; then
    fail "$(relative_artifact_path "$current_report_file") contains unfilled evidence template placeholders (fabrication detected)"
    echo "$report_template_placeholders" | head -5 | sed 's/^/   -> /'
  else
    pass "No unfilled evidence template placeholders in $(relative_artifact_path "$current_report_file")"
  fi
done

# Check 2b: Repo-CLI bypass detection — **Command:** lines must use project CLI, not raw tools
# Auto-detect the project CLI entrypoint (same heuristic as install.sh)
_project_cli=""
for _candidate in ./*.sh; do
  [[ ! -f "$_candidate" ]] && continue
  _base=$(basename "$_candidate")
  case "$_base" in
    install.sh|setup.sh|uninstall.sh|.*.sh) continue ;;
  esac
  _project_cli="./$_base"
  break
done

if [[ -n "$_project_cli" ]]; then
  for current_report_file in "${report_files[@]}"; do
    [[ -f "$current_report_file" ]] || continue
    bypass_commands=""
    while IFS= read -r line; do
      # Match **Command:** `...` lines and extract the command
      if echo "$line" | grep -qE '^\*\*Command:\*\*'; then
        cmd_text="$(echo "$line" | sed -n 's/.*`\(.*\)`.*/\1/p')"
        [[ -z "$cmd_text" ]] && continue
        # Check for forbidden direct tool invocations (must use the project CLI)
        if echo "$cmd_text" | grep -qE '^(go test|go build|go run|cargo test|cargo build|cargo clippy|npm test|npm run|npx jest|npx playwright|node |python |python3 |docker compose|docker-compose)'; then
          bypass_commands="${bypass_commands}   -> $(basename "$current_report_file"): ${cmd_text}"$'\n'
        fi
      fi
    done < "$current_report_file"
    bypass_commands="${bypass_commands%$'\n'}"
    if [[ -n "$bypass_commands" ]]; then
      fail "Report command bypasses repo-standard workflow in $(relative_artifact_path "$current_report_file") (expected: $_project_cli)"
      echo "$bypass_commands"
    else
      pass "No repo-CLI bypass detected in $(relative_artifact_path "$current_report_file") command evidence"
    fi
  done
fi

# Check 3: Evidence legitimacy — code fence blocks must contain real terminal output
# Instead of just counting lines, check for signals that the content is genuine
# terminal/tool output rather than agent-written prose or fabricated summaries.
#
# Terminal output signals (at least 2 required per block):
#   - Test runner patterns: "passed", "failed", "ok", "FAIL", "test result:", "Tests:", "✓", "✗"
#   - Exit/status patterns: "exit code", "Exit Code:", "error[", "warning[", "Compiling", "Finished"
#   - File path patterns: paths with slashes and extensions (src/foo.rs, tests/bar.py)
#   - Timing/metric patterns: "in \d+", "ms", "elapsed", timestamps, durations
#   - Command prompt patterns: "$", ">>>", "root@", lines starting with "running"
#   - Build output: "cargo", "npm", "pytest", "go test", "jest", "playwright", "vite"
#   - HTTP patterns: "HTTP/", "200", "404", "curl", "GET ", "POST ", "Content-Type"
#   - Count patterns: "\d+ passed", "\d+ failed", "0 errors", "0 warnings"
#   - grep/ls output: file listings, permission strings (drwx, -rw-), line-number prefixed output
for current_report_file in "${report_files[@]}"; do
if [[ -f "$current_report_file" ]] && [[ "$state_status" == "done" ]]; then
  evidence_sections_checked=0
  illegitimate_evidence=0
  in_code_block=0
  code_block_lines=0
  code_block_content=""
  code_block_header=""
  while IFS= read -r line; do
    if [[ "$in_code_block" -eq 0 ]] && echo "$line" | grep -qE '^```'; then
      in_code_block=1
      code_block_lines=0
      code_block_content=""
      code_block_header="$prev_line"
    elif [[ "$in_code_block" -eq 1 ]] && echo "$line" | grep -qE '^```$'; then
      in_code_block=0
      evidence_sections_checked=$((evidence_sections_checked + 1))

      # Empty or near-empty blocks are always illegitimate
      if [[ "$code_block_lines" -lt 3 ]]; then
        illegitimate_evidence=$((illegitimate_evidence + 1))
        fail "Evidence block too short ($code_block_lines lines): $(echo "$code_block_header" | head -c 60)"
      else
        # Count terminal output signals in the block content
        terminal_signals=0

        # Test runner output patterns
        if echo "$code_block_content" | grep -qiE '(passed|failed|ok$| PASS | FAIL |test result:|Tests:.*suites|✓|✗|PASSED|FAILED)'; then
          terminal_signals=$((terminal_signals + 1))
        fi

        # Exit/status/compiler patterns
        if echo "$code_block_content" | grep -qiE '(exit code|Exit Code:|error\[|warning\[|Compiling |Finished |error:|warning:|WARN |ERROR |INFO )'; then
          terminal_signals=$((terminal_signals + 1))
        fi

        # File paths with extensions (e.g., src/foo.rs, tests/bar.py, ./path/to/file)
        if echo "$code_block_content" | grep -qE '([a-zA-Z0-9_-]+/[a-zA-Z0-9_.-]+\.(rs|py|ts|tsx|js|go|sh|sql|toml|yaml|json|proto|md)|\./)'; then
          terminal_signals=$((terminal_signals + 1))
        fi

        # Timing/duration/metric patterns
        if echo "$code_block_content" | grep -qiE '(in [0-9]+(\.[0-9]+)?(s|ms|m)|elapsed|finished in|Duration|[0-9]+\.[0-9]+s$)'; then
          terminal_signals=$((terminal_signals + 1))
        fi

        # Build tool / test framework names
        if echo "$code_block_content" | grep -qiE '(cargo |npm |pytest|go test|jest |playwright|vitest|running [0-9]+ test|test result:)'; then
          terminal_signals=$((terminal_signals + 1))
        fi

        # Count/summary patterns (e.g., "12 passed", "0 failed", "3 errors")
        if echo "$code_block_content" | grep -qE '[0-9]+ (passed|failed|errors?|warnings?|skipped|ignored|tests?)'; then
          terminal_signals=$((terminal_signals + 1))
        fi

        # HTTP/curl patterns
        if echo "$code_block_content" | grep -qiE '(HTTP/|status.*[0-9]{3}|curl |GET /|POST /|PUT /|DELETE /|Content-Type)'; then
          terminal_signals=$((terminal_signals + 1))
        fi

        # grep/ls/filesystem output patterns
        if echo "$code_block_content" | grep -qE '(^[dl-][rwx-]{9} |^[0-9]+:|^\$ |^> )'; then
          terminal_signals=$((terminal_signals + 1))
        fi

        # Require at least 2 distinct terminal output signals
        if [[ "$terminal_signals" -lt 2 ]]; then
          illegitimate_evidence=$((illegitimate_evidence + 1))
          fail "Evidence block lacks terminal output signals ($terminal_signals/2 required): $(echo "$code_block_header" | head -c 60)"
        fi
      fi
    elif [[ "$in_code_block" -eq 1 ]]; then
      code_block_lines=$((code_block_lines + 1))
      code_block_content="${code_block_content}${line}"$'\n'
    fi
    prev_line="$line"
  done < "$current_report_file"

  if [[ "$illegitimate_evidence" -eq 0 ]] && [[ "$evidence_sections_checked" -gt 0 ]]; then
    pass "All $evidence_sections_checked evidence blocks in $(relative_artifact_path "$current_report_file") contain legitimate terminal output"
  elif [[ "$evidence_sections_checked" -eq 0 ]]; then
    fail "$(relative_artifact_path "$current_report_file") has no evidence code blocks (status is 'done' but no evidence exists)"
  fi
fi
done

# Check 4: Summary language detection — detect narrative claims without terminal output
# Must track code block state to avoid false positives on genuine terminal output inside ``` blocks
for current_report_file in "${report_files[@]}"; do
if [[ -f "$current_report_file" ]] && [[ "$state_status" == "done" ]]; then
  summary_phrases=""
  in_code=0
  while IFS= read -r line; do
    if [[ "$line" =~ ^\`\`\` ]]; then
      if [[ "$in_code" -eq 0 ]]; then
        in_code=1
      else
        in_code=0
      fi
      continue
    fi
    [[ "$in_code" -eq 1 ]] && continue
    [[ "$line" =~ ^# ]] && continue
    if echo "$line" | grep -iqE '(all tests pass|everything works|no issues found|verified successfully|confirmed working|tests are green|builds successfully|all checks pass)'; then
      summary_phrases+="   -> ${line}"$'\n'
    fi
  done < "$current_report_file"
  summary_phrases="${summary_phrases%$'\n'}"
  if [[ -n "$summary_phrases" ]]; then
    fail "$(relative_artifact_path "$current_report_file") contains narrative summary phrases instead of raw evidence (fabrication indicator)"
    echo "$summary_phrases" | head -5
  else
    pass "No narrative summary phrases detected in $(relative_artifact_path "$current_report_file")"
  fi
fi
done

# Check 5: Specialist completion tracking — all required specialists must be in execution/certification phase records
if [[ -f "$state_file" ]] && [[ "$state_status" == "done" ]] && [[ -n "$state_workflow_mode" ]]; then
  required_specialists=()
  case "$state_workflow_mode" in
    full-delivery|value-first-e2e-batch)
      required_specialists=("implement" "test" "docs" "validate" "audit" "chaos")
      ;;
    full-delivery)
      required_specialists=("implement" "test" "regression" "simplify" "gaps" "harden" "stabilize" "security" "docs" "validate" "audit" "chaos")
      ;;
    feature-bootstrap)
      required_specialists=("implement" "test" "docs" "validate" "audit")
      ;;
    bugfix-fastlane)
      required_specialists=("implement" "test" "validate" "audit")
      ;;
    chaos-hardening)
      required_specialists=("chaos" "implement" "test" "validate" "audit")
      ;;
  esac

  if [[ ${#required_specialists[@]} -gt 0 ]]; then
    for specialist_phase in "${required_specialists[@]}"; do
      if echo "$state_completed_phases_block" | grep -qE "\"$specialist_phase\""; then
        pass "Required specialist phase '$specialist_phase' recorded in execution/certification phase records"
      else
        fail "Required specialist phase '$specialist_phase' NOT in execution/certification phase records (Gate G022 violation)"
      fi
    done
  fi
fi

# Check 5B: Spec review enforcement for legacy-improvement modes
if [[ -f "$state_file" ]] && [[ "$state_status" == "done" ]] && [[ -n "$state_workflow_mode" ]]; then
  spec_review_required="improve-existing|reconcile-to-doc|redesign-existing|full-delivery"
  if echo "$state_workflow_mode" | grep -qE "^($spec_review_required)$"; then
    if echo "$state_completed_phases_block" | grep -qE '"spec-review"'; then
      pass "Spec-review phase recorded for '$state_workflow_mode' (specReview enforcement)"
    else
      fail "'$state_workflow_mode' done status requires spec-review phase but 'spec-review' NOT in completed phases"
    fi
  fi
fi

# Check 6: Duplicate evidence detection — same text in multiple DoD items
for scope_path in "${scope_files[@]}"; do
  [[ -f "$scope_path" ]] || continue
  evidence_blocks=()
  in_evidence=0
  current_evidence=""
  while IFS= read -r line; do
    if [[ "$in_evidence" -eq 0 ]] && echo "$line" | grep -qE '^    ```'; then
      in_evidence=1
      current_evidence=""
    elif [[ "$in_evidence" -eq 1 ]] && echo "$line" | grep -qE '^    ```$'; then
      in_evidence=0
      if [[ -n "$current_evidence" ]]; then
        for prev_evidence in "${evidence_blocks[@]}"; do
          if [[ "$current_evidence" == "$prev_evidence" ]]; then
            fail "Duplicate evidence blocks detected in $(relative_artifact_path "$scope_path") DoD (copy-paste fabrication)"
            break 2
          fi
        done
        evidence_blocks+=("$current_evidence")
      fi
    elif [[ "$in_evidence" -eq 1 ]]; then
      current_evidence="${current_evidence}${line}"
    fi
  done < "$scope_path"
done

echo ""
echo "=== End Anti-Fabrication Checks ==="

if (( failures > 0 )); then
  echo ""
  echo "Artifact lint FAILED with $failures issue(s)."
  fun_message lint_dirty
  exit 1
fi

echo ""
echo "Artifact lint PASSED."
fun_message lint_clean
