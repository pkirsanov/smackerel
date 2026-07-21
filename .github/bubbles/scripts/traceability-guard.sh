#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck source=/dev/null
source "$SCRIPT_DIR/dod-section-lib.sh"

if [[ "${BASH_VERSINFO[0]}" -ge 4 ]]; then
  # shellcheck source=/dev/null
  source "$SCRIPT_DIR/fun-mode.sh"
else
  fun_fail() { :; }
  fun_warn() { :; }
  fun_banner() { :; }
fi

feature_dir="${1:-}"

if [[ -z "$feature_dir" ]]; then
  echo "ERROR: missing feature directory argument"
  echo "Usage: bash bubbles/scripts/traceability-guard.sh specs/<NNN-feature-name> [--all-scopes|--current-scope]"
  exit 2
fi

# Scope-universe mode (BUG-026 C2). Valueless second positional token, mutually
# exclusive, default --all-scopes. Any other form (a value, an =assignment, an
# unknown flag, or a surplus argument) is a hard refusal. Context is derived
# ONLY from state.json via scope-universe-resolver.py — never from an env var.
scope_mode="--all-scopes"
if [[ $# -ge 2 ]]; then
  case "${2:-}" in
    --all-scopes) scope_mode="--all-scopes" ;;
    --current-scope) scope_mode="--current-scope" ;;
    *)
      echo "ERROR: unrecognized second argument: ${2:-}"
      echo "Usage: bash bubbles/scripts/traceability-guard.sh specs/<NNN-feature-name> [--all-scopes|--current-scope]"
      exit 2
      ;;
  esac
fi
if [[ $# -ge 3 ]]; then
  echo "ERROR: too many arguments (expected feature dir and optional --all-scopes|--current-scope)"
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
  if [[ -d "$PWD/$feature_dir" ]]; then
    repo_root="$PWD"
  fi
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
edge_declared=0
edge_inferred=0
edge_ambiguous=0
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

  grep -Eo '"'"$key"'"[[:space:]]*:[[:space:]]*"[^"]+"' "$file" \
    | head -n 1 \
    | sed -E 's/.*"'"$key"'"[[:space:]]*:[[:space:]]*"([^"]+)"/\1/'
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
    output+=("$word")
  done

  printf '%s\n' "${output[@]}"
}

extract_test_rows() {
  local scope_path="$1"
  [[ -f "$scope_path" && -r "$scope_path" ]] || return 2

  awk '
    function without_html_comments(value, result, start_at, end_at) {
      result = ""
      while (length(value) > 0) {
        if (in_html_comment) {
          end_at = index(value, "-->")
          if (end_at == 0) return result
          value = substr(value, end_at + 3)
          in_html_comment = 0
        } else {
          start_at = index(value, "<!--")
          if (start_at == 0) return result value
          result = result substr(value, 1, start_at - 1)
          value = substr(value, start_at + 4)
          in_html_comment = 1
        }
      }
      return result
    }

    function fence_marker(value, stripped, marker, count) {
      stripped = value
      sub(/^[ \t]*/, "", stripped)
      marker = substr(stripped, 1, 1)
      if (marker != "`" && marker != "~") return ""
      count = 0
      while (substr(stripped, count + 1, 1) == marker) count++
      return count >= 3 ? marker : ""
    }

    function atx_heading_depth(value, count, following) {
      count = 0
      while (substr(value, count + 1, 1) == "#") count++
      if (count < 1 || count > 6) return 0
      following = substr(value, count + 1, 1)
      if (following == "" || following == " " || following == "\t") return count
      return 0
    }

    {
      raw = $0
      visible = without_html_comments(raw)

      marker = fence_marker(visible)
      if (in_fence != "") {
        if (marker == in_fence) in_fence = ""
        next
      }
      if (marker != "") {
        in_fence = marker
        next
      }

      candidate = visible
      sub(/[ \t]+$/, "", candidate)

      if (!in_section) {
        if (candidate == "## Test Plan") {
          in_section = 1
          section_depth = 2
          found = 1
        } else if (candidate == "### Test Plan") {
          in_section = 1
          section_depth = 3
          found = 1
        }
        next
      }

      heading_depth = atx_heading_depth(candidate)
      if (heading_depth > 0 && heading_depth <= section_depth) exit

      if (substr(raw, 1, 1) != "|") next
      separator = raw
      gsub(/[|: \t-]/, "", separator)
      if (separator == "") next
      lowered = tolower(raw)
      if (lowered ~ /^\|[ \t]*test type[ \t]*\|/) next
      print raw
    }

    END {
      if (!found) exit 3
    }
  ' "$scope_path"
}

extract_dod_items() {
  local scope_path="$1"
  # BUG-026: route through the shared DoD section parser so the tiered-DoD
  # boundary is correct (nested tier subheadings are retained through depth 6
  # and the section ends only at a same-or-shallower heading) and identical to
  # state-transition Check 4A/22. Emits the checkbox item text after the marker,
  # one per line — the same shape the previous inline awk produced, without the
  # depth-4-tier false boundary that made valid DoDs look rowless (BUG026-F002).
  dod_section_parse "$scope_path" | awk -F'\t' '
    $1 == "CHECKBOX" { out = $4; for (i = 5; i <= NF; i++) out = out "\t" $i; print out }
  '
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
  # and check how many appear in the DoD item.
  #
  # G068 false-positive fix (v3.8.0): percentage-based threshold with floor
  # (see stg_scenario_matches_dod in state-transition-guard.sh for the same
  # logic — both implementations MUST stay aligned).
  # - Very small scenarios (<3 significant words): require ALL words to
  #   match so a hard >=3 floor doesn't penalize them.
  # - Larger scenarios: require BOTH (overlap >= ceil(50% * word_count))
  #   AND (overlap >= 3) — percentage threshold with absolute floor.
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

  if [[ "$word_count" -lt 3 ]]; then
    [[ "$score" -eq "$word_count" ]]
    return
  fi

  threshold=$(( (word_count + 1) / 2 ))
  [[ "$score" -ge 3 && "$score" -ge "$threshold" ]]
}

extract_scenarios() {
  local scope_path="$1"
  grep -E '^[[:space:]]*Scenario( Outline)?:' "$scope_path" | sed -E 's/^[[:space:]]*Scenario( Outline)?:[[:space:]]*//'
}

extract_trace_ids() {
  local value="$1"
  printf '%s\n' "$value" | grep -Eo '(SCN|AC|FR|UC)-[A-Za-z0-9_-]+' || true
}

# classify_match_kind — IMP-015 Scope B (informational only).
# Re-derives the confidence of an already-confirmed scenario→target match
# READ-ONLY: 'declared' iff the scenario's first trace ID also appears in the
# target; otherwise 'inferred'. Never touches failures/warnings/exit.
classify_match_kind() {
  local scenario="$1"
  local target="$2"
  local sid tid
  sid="$(extract_trace_ids "$scenario" | head -n 1 || true)"
  if [[ -n "$sid" ]]; then
    while IFS= read -r tid; do
      [[ -n "$tid" ]] || continue
      if [[ "$tid" == "$sid" ]]; then
        printf 'declared\n'
        return 0
      fi
    done < <(extract_trace_ids "$target")
  fi
  printf 'inferred\n'
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

# BUG-026 C2: in --current-scope mode, resolve the immutable applicable universe
# from state.json (fail-closed) and keep only the applicable scope directories.
# A not_started descendant of the current scope is omitted; the current scope,
# its transitive prerequisites, and applicable siblings are retained.
applicable_scope_dirs=""
if [[ "$scope_mode" == "--current-scope" ]]; then
  if [[ "$scope_layout" != "per-scope-directory" ]]; then
    echo "ERROR: --current-scope requires per-scope-directory layout (single-file scopes.md carries no scopeDir bijection)" >&2
    exit 2
  fi
  scope_resolver="$SCRIPT_DIR/scope-universe-resolver.py"
  if [[ ! -f "$scope_resolver" ]]; then
    echo "ERROR: scope-universe-resolver.py not found (required for --current-scope): $scope_resolver" >&2
    exit 2
  fi
  if ! command -v python3 >/dev/null 2>&1; then
    echo "ERROR: python3 not found (required for --current-scope)" >&2
    exit 2
  fi
  if ! scope_resolver_out="$(python3 "$scope_resolver" "$feature_dir" current-scope 2>&1)"; then
    echo "ERROR: scope-universe resolution refused (--current-scope):" >&2
    printf '%s\n' "$scope_resolver_out" >&2
    exit 2
  fi
  applicable_scope_dirs="$(printf '%s\n' "$scope_resolver_out" \
    | awk -F'\t' '$1=="RECORD" && $6=="true" && $7!="" { n=split($7,a,"/"); print a[n] }')"
  if [[ -z "$applicable_scope_dirs" ]]; then
    echo "ERROR: --current-scope resolved an empty applicable universe (no scopeDir on any applicable state record)" >&2
    exit 2
  fi
fi

if [[ "$scope_layout" == "per-scope-directory" ]]; then
  while IFS= read -r scope_path; do
    if [[ "$scope_mode" == "--current-scope" ]]; then
      scope_local_dir="$(basename "$(dirname "$scope_path")")"
      if ! printf '%s\n' "$applicable_scope_dirs" | grep -qxF "$scope_local_dir"; then
        continue
      fi
    fi
    scope_files+=("$scope_path")
  done < <(find "$feature_dir/scopes" -mindepth 2 -maxdepth 2 -type f -name 'scope.md' | sort)
else
  scope_files+=("$feature_dir/scopes.md")
fi

if [[ "$scope_mode" == "--current-scope" && ${#scope_files[@]} -eq 0 ]]; then
  echo "ERROR: --current-scope matched no physical scope directory (state scopeDir vs disk name mismatch)" >&2
  exit 2
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

  scenarios=""
  scenario_status=0
  if scenarios="$(extract_scenarios "$scope_path")"; then
    scenario_status=0
  else
    scenario_status=$?
  fi

  if [[ "$scenario_status" -eq 1 ]] || [[ -z "$scenarios" ]]; then
    fail "$scope_label has no Gherkin scenarios to trace"
    continue
  elif [[ "$scenario_status" -ne 0 ]]; then
    fail "$scope_label Gherkin scenario extraction failed"
    continue
  fi

  test_rows=""
  test_rows_status=0
  if test_rows="$(extract_test_rows "$scope_path")"; then
    test_rows_status=0
  else
    test_rows_status=$?
  fi

  if [[ "$test_rows_status" -eq 3 ]]; then
    fail "$scope_label has no recognized Test Plan section (expected exact ## Test Plan or ### Test Plan)"
    continue
  elif [[ "$test_rows_status" -ne 0 ]]; then
    fail "$scope_label Test Plan extraction failed"
    continue
  elif [[ -z "$test_rows" ]]; then
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

    edge_kind="$(classify_match_kind "$scenario" "$matched_row")"
    if [[ "$edge_kind" == "inferred" ]]; then
      _amb=0
      while IFS= read -r _r; do
        [[ -n "$_r" ]] || continue
        if scenario_matches_row "$scenario" "$_r"; then
          _amb=$((_amb + 1))
        fi
      done <<< "$test_rows"
      [[ "$_amb" -gt 1 ]] && edge_kind="ambiguous"
    fi
    case "$edge_kind" in
      declared) edge_declared=$((edge_declared + 1)) ;;
      ambiguous) edge_ambiguous=$((edge_ambiguous + 1)) ;;
      *) edge_inferred=$((edge_inferred + 1)) ;;
    esac
    info "$scope_label scenario→row match confidence: $edge_kind"

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
  scenarios=""
  scenario_status=0
  if scenarios="$(extract_scenarios "$scope_path")"; then
    scenario_status=0
  else
    scenario_status=$?
  fi
  dod_items="$(extract_dod_items "$scope_path")"

  if [[ "$scenario_status" -eq 1 ]] || [[ -z "$scenarios" ]]; then
    continue
  elif [[ "$scenario_status" -ne 0 ]]; then
    fail "$scope_label Gherkin scenario extraction failed"
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
      edge_kind="$(classify_match_kind "$scenario" "$matched_dod")"
      if [[ "$edge_kind" == "inferred" ]]; then
        _amb=0
        while IFS= read -r _d; do
          [[ -n "$_d" ]] || continue
          if scenario_matches_dod "$scenario" "$_d"; then
            _amb=$((_amb + 1))
          fi
        done <<< "$dod_items"
        [[ "$_amb" -gt 1 ]] && edge_kind="ambiguous"
      fi
      case "$edge_kind" in
        declared) edge_declared=$((edge_declared + 1)) ;;
        ambiguous) edge_ambiguous=$((edge_ambiguous + 1)) ;;
        *) edge_inferred=$((edge_inferred + 1)) ;;
      esac
      info "$scope_label scenario→DoD match confidence: $edge_kind"
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
info "Edge confidence (IMP-015 Scope B): declared=$edge_declared inferred=$edge_inferred ambiguous=$edge_ambiguous"

if [[ "$failures" -gt 0 ]]; then
  echo ""
  echo "RESULT: FAILED ($failures failures, $warnings warnings)"
  exit 1
fi

echo ""
echo "RESULT: PASSED ($warnings warnings)"
exit 0
