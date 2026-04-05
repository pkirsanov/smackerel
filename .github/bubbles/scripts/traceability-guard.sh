#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

source "$SCRIPT_DIR/fun-mode.sh"

feature_dir="${1:-}"

if [[ -z "$feature_dir" ]]; then
  echo "ERROR: missing feature directory argument"
  echo "Usage: bash bubbles/scripts/traceability-guard.sh specs/<NNN-feature-name>"
  exit 2
fi

detect_repo_root() {
  if [[ "$SCRIPT_DIR" == */.github/bubbles/scripts ]]; then
    (cd "$SCRIPT_DIR/../../.." && pwd)
  else
    (cd "$SCRIPT_DIR/../.." && pwd)
  fi
}

repo_root="$(detect_repo_root)"

if [[ "$feature_dir" != /* ]]; then
  feature_dir="$repo_root/$feature_dir"
fi

if [[ ! -d "$feature_dir" ]]; then
  echo "ERROR: feature directory not found: $feature_dir"
  exit 2
fi

failures=0
warnings=0
scenario_total=0
row_total=0
mapped_total=0
file_reference_total=0
report_reference_total=0
scenario_manifest_total=0
scenario_manifest_file="$feature_dir/scenario-manifest.json"

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
  warnings=$((warnings + 1))
}

pass() {
  local message="$1"
  echo "✅ $message"
}

info() {
  local message="$1"
  echo "ℹ️  $message"
}

json_first_string() {
  local key="$1"
  local file="$2"
  if [[ ! -f "$file" ]]; then
    return 0
  fi

  grep -Eo '"'"'"$key"'"'"[[:space:]]*:[[:space:]]*"[^"]+"' "$file" \
    | head -n 1 \
    | sed -E 's/.*"'"'"$key"'"'"[[:space:]]*:[[:space:]]*"([^"]+)"/\1/'
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

scope_section_tmp_files=()

cleanup_tmp_artifacts() {
  if [[ ${#scope_section_tmp_files[@]} -gt 0 ]]; then
    rm -f "${scope_section_tmp_files[@]}"
  fi
}

trap cleanup_tmp_artifacts EXIT

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

normalize_text() {
  local value="$1"
  value="$(printf '%s' "$value" | tr '[:upper:]' '[:lower:]')"
  value="$(printf '%s' "$value" | sed -E 's/[^a-z0-9]+/ /g; s/[[:space:]]+/ /g; s/^ //; s/ $//')"
  printf '%s' "$value"
}

significant_words() {
  local text="$1"
  local normalized
  local word
  local output=()

  normalized="$(normalize_text "$text")"
  for word in $normalized; do
    if [[ ${#word} -lt 4 ]]; then
      continue
    fi
    case "$word" in
      given|when|then|with|from|into|onto|that|this|those|these|user|users|system|should|must|have|has|been|were|will|after|before|while|where|their|there|about|only|each)
        continue
        ;;
    esac
    output+=("$word")
  done

  printf '%s\n' "${output[@]}"
}

extract_section() {
  local file_path="$1"
  local heading_regex="$2"
  awk -v heading_regex="$heading_regex" '
    $0 ~ heading_regex {in_section=1; next}
    /^### / {if (in_section) exit}
    in_section {print}
  ' "$file_path"
}

extract_test_rows() {
  local scope_path="$1"
  local section
  section="$(extract_section "$scope_path" '^### Test Plan')"
  printf '%s\n' "$section" \
    | grep -E '^\|' \
    | grep -Ev '^\|[-:[:space:]|]+\|$' \
    | grep -Evi '^\|[[:space:]]*test type[[:space:]]*\|'
}

extract_dod_items() {
  local scope_path="$1"
  awk '
    /^#{1,4}.*Definition of Done|^#{1,4}.*DoD/ {in_dod=1; next}
    /^#{1,4} / {if (in_dod) exit}
    in_dod && /^- \[(x| )\] / {
      sub(/^- \[(x| )\] /, "", $0)
      print
    }
  ' "$scope_path"
}

scenario_matches_dod() {
  local scenario="$1"
  local dod_item="$2"
  local scenario_id
  local dod_id
  local words
  local word
  local dod_norm
  local score=0
  local threshold=0
  local word_count=0

  # Try trace ID matching first
  scenario_id="$(extract_trace_ids "$scenario" | head -n 1 || true)"
  if [[ -n "$scenario_id" ]]; then
    while IFS= read -r dod_id; do
      if [[ -n "$dod_id" ]] && [[ "$dod_id" == "$scenario_id" ]]; then
        return 0
      fi
    done < <(extract_trace_ids "$dod_item")
  fi

  # Fuzzy word matching — extract significant words from the scenario
  # and check how many appear in the DoD item
  dod_norm="$(normalize_text "$dod_item")"
  words="$(significant_words "$scenario")"
  if [[ -z "$words" ]]; then
    [[ "$dod_norm" == *"$(normalize_text "$scenario")"* ]]
    return
  fi

  while IFS= read -r word; do
    [[ -n "$word" ]] || continue
    word_count=$((word_count + 1))
    if [[ " $dod_norm " == *" $word "* ]]; then
      score=$((score + 1))
    fi
  done <<< "$words"

  if [[ "$word_count" -le 1 ]]; then
    threshold=1
  elif [[ "$word_count" -le 3 ]]; then
    threshold=2
  else
    threshold=3
  fi

  [[ "$score" -ge "$threshold" ]]
}

extract_scenarios() {
  local scope_path="$1"
  grep -E '^[[:space:]]*Scenario( Outline)?:' "$scope_path" | sed -E 's/^[[:space:]]*Scenario( Outline)?:[[:space:]]*//'
}

extract_trace_ids() {
  local value="$1"
  printf '%s\n' "$value" | grep -Eo '(SCN|AC|FR|UC)-[A-Za-z0-9_-]+' || true
}

extract_path_candidates() {
  local value="$1"
  printf '%s\n' "$value" | grep -Eo '([A-Za-z0-9_.-]+/)+[A-Za-z0-9_.-]+\.[A-Za-z0-9_.-]+' || true
}

path_exists() {
  local candidate="$1"
  local scope_dir="$2"

  if [[ -f "$repo_root/$candidate" ]]; then
    return 0
  fi

  if [[ -f "$scope_dir/$candidate" ]]; then
    return 0
  fi

  return 1
}

report_mentions_path() {
  local report_path="$1"
  local candidate="$2"

  if [[ ! -f "$report_path" ]]; then
    return 1
  fi

  grep -Fq "$candidate" "$report_path"
}

scenario_matches_row() {
  local scenario="$1"
  local row="$2"
  local scenario_id
  local row_id
  local words
  local word
  local row_norm
  local score=0
  local threshold=0
  local word_count=0

  scenario_id="$(extract_trace_ids "$scenario" | head -n 1 || true)"
  if [[ -n "$scenario_id" ]]; then
    while IFS= read -r row_id; do
      if [[ -n "$row_id" ]] && [[ "$row_id" == "$scenario_id" ]]; then
        return 0
      fi
    done < <(extract_trace_ids "$row")
  fi

  row_norm="$(normalize_text "$row")"
  words="$(significant_words "$scenario")"
  if [[ -z "$words" ]]; then
    [[ "$row_norm" == *"$(normalize_text "$scenario")"* ]]
    return
  fi

  while IFS= read -r word; do
    [[ -n "$word" ]] || continue
    word_count=$((word_count + 1))
    if [[ " $row_norm " == *" $word "* ]]; then
      score=$((score + 1))
    fi
  done <<< "$words"

  if [[ "$word_count" -le 1 ]]; then
    threshold=1
  else
    threshold=2
  fi

  [[ "$score" -ge "$threshold" ]]
}

scope_layout="$(detect_scope_layout)"
scope_files=()
scope_analysis_files=()
scope_analysis_labels=()

if [[ "$scope_layout" == "per-scope-directory" ]]; then
  while IFS= read -r scope_path; do
    scope_files+=("$scope_path")
  done < <(find "$feature_dir/scopes" -mindepth 2 -maxdepth 2 -type f -name 'scope.md' | sort)
else
  scope_files+=("$feature_dir/scopes.md")
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

echo "============================================================"
echo "  BUBBLES TRACEABILITY GUARD"
echo "  Feature: $feature_dir"
echo "  Timestamp: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
echo "============================================================"
fun_banner
echo ""

echo "--- Scenario Manifest Cross-Check (G057/G059) ---"
scope_defined_scenarios=0
for scope_path in "${scope_files[@]}"; do
  [[ -f "$scope_path" ]] || continue
  scope_defined_scenarios=$((scope_defined_scenarios + $(grep -cE '^[[:space:]]*Scenario( Outline)?:' "$scope_path" || true)))
done

if [[ "$scope_defined_scenarios" -gt 0 ]]; then
  if [[ ! -f "$scenario_manifest_file" ]]; then
    fail "Resolved scopes define $scope_defined_scenarios Gherkin scenarios but scenario-manifest.json is missing"
  else
    scenario_manifest_total="$(grep -cE '"scenarioId"[[:space:]]*:' "$scenario_manifest_file" || true)"
    if [[ "$scenario_manifest_total" -lt "$scope_defined_scenarios" ]]; then
      fail "scenario-manifest.json covers only $scenario_manifest_total scenarios but scopes define $scope_defined_scenarios"
    else
      pass "scenario-manifest.json covers $scenario_manifest_total scenario contract(s)"
    fi

    manifest_missing_files=0
    while IFS= read -r manifest_test_file; do
      [[ -n "$manifest_test_file" ]] || continue
      if path_exists "$manifest_test_file" "$feature_dir"; then
        pass "scenario-manifest.json linked test exists: $manifest_test_file"
      else
        fail "scenario-manifest.json references missing linked test file: $manifest_test_file"
        manifest_missing_files=$((manifest_missing_files + 1))
      fi
    done < <(grep -Eo '"file"[[:space:]]*:[[:space:]]*"[^"]+"' "$scenario_manifest_file" 2>/dev/null | sed -E 's/.*:[[:space:]]*"([^"]+)"/\1/' || true)

    if grep -qE '"evidenceRefs"[[:space:]]*:[[:space:]]*\[' "$scenario_manifest_file"; then
      pass "scenario-manifest.json records evidenceRefs"
    else
      fail "scenario-manifest.json is missing evidenceRefs entries"
    fi

    if [[ "$manifest_missing_files" -eq 0 ]]; then
      pass "All linked tests from scenario-manifest.json exist"
    fi
  fi
else
  info "No scope-defined Gherkin scenarios found — scenario manifest cross-check skipped"
fi
echo ""

for scope_index in "${!scope_analysis_files[@]}"; do
  scope_path="${scope_analysis_files[$scope_index]}"
  if [[ ! -f "$scope_path" ]]; then
    fail "Missing scope file: $(scope_analysis_label "$scope_index")"
    continue
  fi

  scope_label="$(scope_analysis_label "$scope_index")"
  scope_dir="$(dirname "$scope_path")"
  if [[ "$scope_layout" == "per-scope-directory" ]]; then
    report_path="$scope_dir/report.md"
  else
    report_path="$feature_dir/report.md"
  fi

  info "Checking traceability for $scope_label"

  scenarios="$(extract_scenarios "$scope_path")"
  test_rows="$(extract_test_rows "$scope_path")"

  if [[ -z "$scenarios" ]]; then
    fail "$scope_label has no Gherkin scenarios to trace"
    continue
  fi

  if [[ -z "$test_rows" ]]; then
    fail "$scope_label has no concrete Test Plan rows to trace"
    continue
  fi

  scope_scenario_count=0
  scope_row_count=0
  while IFS= read -r _row; do
    [[ -n "$_row" ]] || continue
    scope_row_count=$((scope_row_count + 1))
  done <<< "$test_rows"

  row_total=$((row_total + scope_row_count))

  while IFS= read -r scenario; do
    [[ -n "$scenario" ]] || continue
    scope_scenario_count=$((scope_scenario_count + 1))
    scenario_total=$((scenario_total + 1))

    matched_row=""
    while IFS= read -r row; do
      [[ -n "$row" ]] || continue
      if scenario_matches_row "$scenario" "$row"; then
        matched_row="$row"
        break
      fi
    done <<< "$test_rows"

    if [[ -z "$matched_row" ]]; then
      fail "$scope_label scenario has no traceable Test Plan row: $scenario"
      continue
    fi

    mapped_total=$((mapped_total + 1))
    pass "$scope_label scenario mapped to Test Plan row: $scenario"

    path_candidates="$(extract_path_candidates "$matched_row")"
    if [[ -z "$path_candidates" ]]; then
      fail "$scope_label mapped row has no concrete test file path: $scenario"
      continue
    fi

    existing_path=""
    while IFS= read -r candidate; do
      [[ -n "$candidate" ]] || continue
      if path_exists "$candidate" "$scope_dir"; then
        existing_path="$candidate"
        break
      fi
    done <<< "$path_candidates"

    if [[ -z "$existing_path" ]]; then
      fail "$scope_label mapped row references no existing concrete test file: $scenario"
      continue
    fi

    file_reference_total=$((file_reference_total + 1))
    pass "$scope_label scenario maps to concrete test file: $existing_path"

    if report_mentions_path "$report_path" "$existing_path"; then
      report_reference_total=$((report_reference_total + 1))
      pass "$scope_label report references concrete test evidence: $existing_path"
    else
      fail "$scope_label report is missing evidence reference for concrete test file: $existing_path"
    fi
  done <<< "$scenarios"

  info "$scope_label summary: scenarios=$scope_scenario_count test_rows=$scope_row_count"
  echo ""
done

# =============================================================================
# PASS 2: Gherkin → DoD Content Fidelity (Gate G068)
# =============================================================================
# Verifies that every Gherkin scenario's behavioral claim is faithfully
# represented by at least one DoD item. Detects the failure mode where DoD
# items are silently rewritten to match delivery instead of the spec.
# =============================================================================
echo "--- Gherkin → DoD Content Fidelity (Gate G068) ---"
dod_fidelity_total=0
dod_fidelity_mapped=0
dod_fidelity_unmapped=0

for scope_index in "${!scope_analysis_files[@]}"; do
  scope_path="${scope_analysis_files[$scope_index]}"
  [[ -f "$scope_path" ]] || continue

  scope_label="$(scope_analysis_label "$scope_index")"
  scenarios="$(extract_scenarios "$scope_path")"
  dod_items="$(extract_dod_items "$scope_path")"

  if [[ -z "$scenarios" ]]; then
    continue
  fi

  if [[ -z "$dod_items" ]]; then
    fail "$scope_label has Gherkin scenarios but no DoD items — cannot verify content fidelity"
    continue
  fi

  while IFS= read -r scenario; do
    [[ -n "$scenario" ]] || continue
    dod_fidelity_total=$((dod_fidelity_total + 1))

    matched_dod=""
    while IFS= read -r dod_item; do
      [[ -n "$dod_item" ]] || continue
      if scenario_matches_dod "$scenario" "$dod_item"; then
        matched_dod="$dod_item"
        break
      fi
    done <<< "$dod_items"

    if [[ -z "$matched_dod" ]]; then
      fail "$scope_label Gherkin scenario has no faithful DoD item preserving its behavioral claim: $scenario"
      dod_fidelity_unmapped=$((dod_fidelity_unmapped + 1))
    else
      dod_fidelity_mapped=$((dod_fidelity_mapped + 1))
      pass "$scope_label scenario maps to DoD item: $scenario"
    fi
  done <<< "$scenarios"
done

if [[ "$dod_fidelity_total" -gt 0 ]]; then
  info "DoD fidelity: $dod_fidelity_total scenarios checked, $dod_fidelity_mapped mapped to DoD, $dod_fidelity_unmapped unmapped"
  if [[ "$dod_fidelity_unmapped" -gt 0 ]]; then
    fail "DoD content fidelity gap: $dod_fidelity_unmapped Gherkin scenario(s) have no matching DoD item — DoD may have been rewritten to match delivery instead of the spec (Gate G068)"
  fi
else
  info "No scenarios to check for DoD content fidelity"
fi
echo ""

echo "--- Traceability Summary ---"
info "Scenarios checked: $scenario_total"
info "Test rows checked: $row_total"
info "Scenario-to-row mappings: $mapped_total"
info "Concrete test file references: $file_reference_total"
info "Report evidence references: $report_reference_total"
info "DoD fidelity scenarios: $dod_fidelity_total (mapped: $dod_fidelity_mapped, unmapped: $dod_fidelity_unmapped)"

if [[ "$failures" -gt 0 ]]; then
  echo ""
  echo "RESULT: FAILED ($failures failures, $warnings warnings)"
  exit 1
fi

echo ""
echo "RESULT: PASSED ($warnings warnings)"
exit 0
