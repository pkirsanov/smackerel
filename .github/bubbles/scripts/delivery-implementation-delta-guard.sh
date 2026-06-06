#!/usr/bin/env bash
set -euo pipefail

# delivery-implementation-delta-guard.sh
#
# Gate G093 - delivery_implementation_delta_gate.
#
# Done-ceiling delivery modes must prove at least one delivery delta outside
# planning bookkeeping paths (`specs/**` and `.specify/**`). G053 remains the
# report-shape/code-diff evidence gate; this guard owns status-ceiling-aware
# changed-path classification and blocked owner routing.
#
# Usage:
#   bash bubbles/scripts/delivery-implementation-delta-guard.sh <specDir> [--quiet] [--base <ref>] [--head <ref>]
#
# Exit codes:
#   0  clean or lower-ceiling exemption
#   1  G093 delivery implementation delta violation
#   2  runtime error, invalid arguments, missing state, malformed JSON, or bad refs

QUIET="false"
SPEC_DIR_INPUT=""
BASE_REF=""
HEAD_REF=""

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/delivery-implementation-delta-guard.sh <specDir> [--quiet] [--base <ref>] [--head <ref>]

Arguments:
  <specDir>     Spec directory containing state.json.

Optional:
  --quiet       Suppress success/skip output.
  --base <ref>  Base git ref for the certification window.
  --head <ref>  Head git ref for the certification window.
  -h, --help    Print this usage and exit.

Exit codes:
  0 = clean or exempt because workflow statusCeiling is below done
  1 = G093 delivery implementation delta violation
  2 = runtime error, invalid arguments, missing state, malformed JSON, or bad refs
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --quiet)
      QUIET="true"
      shift
      ;;
    --base)
      [[ $# -ge 2 ]] || { echo "delivery-implementation-delta-guard: --base requires a value" >&2; exit 2; }
      BASE_REF="$2"
      shift 2
      ;;
    --head)
      [[ $# -ge 2 ]] || { echo "delivery-implementation-delta-guard: --head requires a value" >&2; exit 2; }
      HEAD_REF="$2"
      shift 2
      ;;
    --*)
      echo "delivery-implementation-delta-guard: unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -n "$SPEC_DIR_INPUT" ]]; then
        echo "delivery-implementation-delta-guard: only one specDir may be supplied" >&2
        usage >&2
        exit 2
      fi
      SPEC_DIR_INPUT="$1"
      shift
      ;;
  esac
done

if [[ -z "$SPEC_DIR_INPUT" ]]; then
  echo "delivery-implementation-delta-guard: missing required specDir" >&2
  usage >&2
  exit 2
fi

if [[ -n "$BASE_REF" && -z "$HEAD_REF" ]] || [[ -z "$BASE_REF" && -n "$HEAD_REF" ]]; then
  echo "delivery-implementation-delta-guard: --base and --head must be supplied together" >&2
  exit 2
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "delivery-implementation-delta-guard: jq is required but not found in PATH" >&2
  exit 2
fi

if [[ ! -d "$SPEC_DIR_INPUT" ]]; then
  echo "delivery-implementation-delta-guard: specDir does not exist: $SPEC_DIR_INPUT" >&2
  exit 2
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SPEC_DIR_ABS="$(cd "$SPEC_DIR_INPUT" && pwd -P)"
STATE_FILE="$SPEC_DIR_ABS/state.json"

resolve_repo_file() {
  local rel="$1"
  local candidate

  for candidate in "$REPO_ROOT/$rel" "$REPO_ROOT/.github/$rel"; do
    if [[ -f "$candidate" ]]; then
      printf '%s' "$candidate"
      return 0
    fi
  done

  return 1
}

if [[ ! -f "$STATE_FILE" ]]; then
  echo "delivery-implementation-delta-guard: state.json not found: $STATE_FILE" >&2
  exit 2
fi

if ! jq -e 'type == "object"' "$STATE_FILE" >/dev/null 2>&1; then
  echo "delivery-implementation-delta-guard: malformed or non-object JSON: $STATE_FILE" >&2
  exit 2
fi

resolve_repo_root() {
  if [[ -n "${BUBBLES_REPO_ROOT:-}" ]]; then
    (cd "$BUBBLES_REPO_ROOT" && pwd -P)
    return 0
  fi

  local parent
  parent="$(dirname "$SPEC_DIR_ABS")"
  if [[ "$(basename "$parent")" == "specs" ]]; then
    (cd "$(dirname "$parent")" && pwd -P)
    return 0
  fi

  local dir
  dir="$SPEC_DIR_ABS"
  while [[ "$dir" != "/" ]]; do
    if [[ -d "$dir/specs" && ( -f "$dir/bubbles/workflows.yaml" || -f "$dir/.github/bubbles/workflows.yaml" || -d "$dir/.specify/memory" ) ]]; then
      printf '%s\n' "$dir"
      return 0
    fi
    dir="$(dirname "$dir")"
  done

  return 1
}

REPO_ROOT="$(resolve_repo_root || true)"
if [[ -z "$REPO_ROOT" || ! -d "$REPO_ROOT" ]]; then
  echo "delivery-implementation-delta-guard: unable to resolve repository root from specDir: $SPEC_DIR_INPUT" >&2
  exit 2
fi

WORKFLOWS_FILE="$(resolve_repo_file "bubbles/workflows.yaml" || true)"
if [[ -z "$WORKFLOWS_FILE" ]]; then
  echo "delivery-implementation-delta-guard: workflows.yaml not found under $REPO_ROOT/bubbles or $REPO_ROOT/.github/bubbles" >&2
  exit 2
fi

# v6.1 (S2 true split): mode definitions live in bubbles/workflows/modes.yaml,
# not inside workflows.yaml. Raw .modes reads use this file; .modeTemplates
# reads stay on workflows.yaml. Fall back to workflows.yaml for pre-split or
# transitional repos that still embed an inline modes: block.
MODES_FILE="$(dirname "$WORKFLOWS_FILE")/workflows/modes.yaml"
[[ -f "$MODES_FILE" ]] || MODES_FILE="$WORKFLOWS_FILE"

relative_path() {
  local path="$1"
  if [[ "$path" == "$REPO_ROOT" ]]; then
    printf '.'
  elif [[ "$path" == "$REPO_ROOT"/* ]]; then
    printf '%s' "${path#$REPO_ROOT/}"
  else
    printf '%s' "$path"
  fi
}

spec_rel="$(relative_path "$SPEC_DIR_ABS")"

state_string() {
  local expr="$1"
  jq -r "$expr" "$STATE_FILE"
}

state_workflow_mode="$(state_string '.workflowMode // ""')"
state_status="$(state_string '.status // ""')"
planning_only="$(state_string 'if .planningOnly == true then "true" else "false" end')"
planning_only_justification="$(state_string '.planningOnlyJustification // ""')"

if [[ -z "$state_workflow_mode" ]]; then
  echo "delivery-implementation-delta-guard: state.workflowMode is required in $STATE_FILE" >&2
  exit 2
fi

resolve_workflow_status_ceiling() {
  local workflow_mode="$1"
  local resolver="$SCRIPT_DIR/mode-resolver.sh"
  local resolved=""
  local status_ceiling=""

  status_ceiling="$(awk -v mode="$workflow_mode" '
    /^[[:space:]]*modes:[[:space:]]*$/ { in_modes = 1; next }
    in_modes && /^[^[:space:]#][^:]*:[[:space:]]*$/ { in_modes = 0 }
    !in_modes { next }
    /^  [^[:space:]#][^:]*:[[:space:]]*$/ {
      current = $0
      sub(/^  /, "", current)
      sub(/:.*/, "", current)
      in_mode = (current == mode)
      next
    }
    in_mode && /^    statusCeiling:[[:space:]]*/ {
      value = $0
      sub(/^    statusCeiling:[[:space:]]*/, "", value)
      gsub(/"/, "", value)
      gsub(/[[:space:]]/, "", value)
      print value
      exit
    }
  ' "$WORKFLOWS_FILE" 2>/dev/null || true)"

  if [[ -n "$status_ceiling" && "$status_ceiling" != "null" ]]; then
    printf '%s\n' "$status_ceiling"
    return 0
  fi

  if [[ -f "$resolver" ]]; then
    # v7: this resolves a PERSISTED workflowMode from an existing artifact, so
    # grandfather the stored (possibly v5-name) registry key — the resolver
    # rejects bare v5 names only for new operator input.
    if resolved="$(BUBBLES_MODE_GRANDFATHER=1 BUBBLES_WORKFLOWS_FILE="$WORKFLOWS_FILE" bash "$resolver" "$workflow_mode" 2>/dev/null)"; then
      status_ceiling="$(printf '%s\n' "$resolved" | awk -F':[[:space:]]*' '{ key=$1; gsub(/^[[:space:]]+|[[:space:]]+$/, "", key); if (key == "statusCeiling") { gsub(/"/, "", $2); print $2; exit } }')"
    fi
  fi

  if [[ -z "$status_ceiling" ]] && command -v yq >/dev/null 2>&1; then
    status_ceiling="$(yq -r ".modes.\"$workflow_mode\".statusCeiling // \"\"" "$MODES_FILE" 2>/dev/null || true)"
    if [[ -z "$status_ceiling" || "$status_ceiling" == "null" ]]; then
      local inherited_template=""
      while IFS= read -r inherited_template; do
        [[ -n "$inherited_template" ]] || continue
        status_ceiling="$(yq -r ".modeTemplates.\"$inherited_template\".statusCeiling // \"\"" "$WORKFLOWS_FILE" 2>/dev/null || true)"
        if [[ -n "$status_ceiling" && "$status_ceiling" != "null" ]]; then
          break
        fi
      done < <(yq -r ".modes.\"$workflow_mode\".inherits[]?" "$MODES_FILE" 2>/dev/null || true)
    fi
  fi

  if [[ -z "$status_ceiling" || "$status_ceiling" == "null" ]]; then
    return 1
  fi

  printf '%s\n' "$status_ceiling"
}

status_ceiling="$(resolve_workflow_status_ceiling "$state_workflow_mode" || true)"
if [[ -z "$status_ceiling" ]]; then
  echo "delivery-implementation-delta-guard: unable to resolve statusCeiling for workflowMode '$state_workflow_mode' from $WORKFLOWS_FILE" >&2
  exit 2
fi

changed_paths=()
git_path_events=0
report_path_events=0
report_code_diff_sections=0

add_changed_path() {
  local raw_path="$1"
  local source="$2"
  local path="$raw_path"

  path="${path//$'\r'/}"
  path="${path#./}"
  path="${path%%#*}"
  path="${path%/}"

  path="$(printf '%s' "$path" | sed -E 's/^[`"({[,]+//; s/[]`")},.;:]+$//')"

  path="${path#path=}"
  path="${path#file=}"
  path="${path#changed=}"
  path="${path#A:}"
  path="${path#M:}"
  path="${path#D:}"

  [[ -n "$path" ]] || return 0
  [[ "$path" != http://* && "$path" != https://* ]] || return 0

  if [[ "$path" == "$REPO_ROOT"/* ]]; then
    path="${path#$REPO_ROOT/}"
  fi

  [[ "$path" == */* || "$path" == README.md || "$path" == CHANGELOG.md || "$path" == Dockerfile* || "$path" == Makefile ]] || return 0

  local existing
  for existing in "${changed_paths[@]}"; do
    if [[ "$existing" == "$path" ]]; then
      if [[ "$source" == "git" ]]; then
        git_path_events=$((git_path_events + 1))
      elif [[ "$source" == "report" ]]; then
        report_path_events=$((report_path_events + 1))
      fi
      return 0
    fi
  done

  changed_paths+=("$path")
  if [[ "$source" == "git" ]]; then
    git_path_events=$((git_path_events + 1))
  elif [[ "$source" == "report" ]]; then
    report_path_events=$((report_path_events + 1))
  fi
}

add_paths_from_lines() {
  local source="$1"
  local text="$2"
  local line token

  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    for token in $line; do
      add_changed_path "$token" "$source"
    done
  done <<< "$text"
}

collect_git_paths() {
  local output=""
  local status_output=""
  local status_path=""

  if ! git -C "$REPO_ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    return 0
  fi

  if [[ -n "$BASE_REF" && -n "$HEAD_REF" ]]; then
    if ! git -C "$REPO_ROOT" rev-parse --verify "$BASE_REF^{commit}" >/dev/null 2>&1; then
      echo "delivery-implementation-delta-guard: --base ref is not a commit: $BASE_REF" >&2
      exit 2
    fi
    if ! git -C "$REPO_ROOT" rev-parse --verify "$HEAD_REF^{commit}" >/dev/null 2>&1; then
      echo "delivery-implementation-delta-guard: --head ref is not a commit: $HEAD_REF" >&2
      exit 2
    fi
    output="$(git -C "$REPO_ROOT" diff --name-only "$BASE_REF" "$HEAD_REF" -- 2>/dev/null || true)"
    add_paths_from_lines "git" "$output"
    return 0
  fi

  output="$(git -C "$REPO_ROOT" diff --name-only HEAD -- 2>/dev/null || true)"
  add_paths_from_lines "git" "$output"
  output="$(git -C "$REPO_ROOT" diff --cached --name-only -- 2>/dev/null || true)"
  add_paths_from_lines "git" "$output"
  output="$(git -C "$REPO_ROOT" diff --name-only -- 2>/dev/null || true)"
  add_paths_from_lines "git" "$output"

  status_output="$(git -C "$REPO_ROOT" status --short --untracked-files=all 2>/dev/null || true)"
  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    status_path="${line:3}"
    if [[ "$status_path" == *" -> "* ]]; then
      status_path="${status_path##* -> }"
    fi
    add_changed_path "$status_path" "git"
  done <<< "$status_output"
}

extract_code_diff_section() {
  local report_file="$1"
  awk '
    /^#{2,6}[[:space:]]+Code Diff Evidence([[:space:]]|$)/ { in_section = 1; next }
    in_section && /^#{1,6}[[:space:]]+/ { in_section = 0 }
    in_section { print }
  ' "$report_file"
}

collect_report_paths() {
  local report_file=""
  local section_text=""

  if [[ -f "$SPEC_DIR_ABS/report.md" ]]; then
    if grep -qE '^#{2,6}[[:space:]]+Code Diff Evidence([[:space:]]|$)' "$SPEC_DIR_ABS/report.md"; then
      report_code_diff_sections=$((report_code_diff_sections + 1))
      section_text="$(extract_code_diff_section "$SPEC_DIR_ABS/report.md")"
      add_paths_from_lines "report" "$section_text"
    fi
  fi

  if [[ -d "$SPEC_DIR_ABS/scopes" ]]; then
    while IFS= read -r report_file; do
      [[ -f "$report_file" ]] || continue
      if grep -qE '^#{2,6}[[:space:]]+Code Diff Evidence([[:space:]]|$)' "$report_file"; then
        report_code_diff_sections=$((report_code_diff_sections + 1))
        section_text="$(extract_code_diff_section "$report_file")"
        add_paths_from_lines "report" "$section_text"
      fi
    done < <(find "$SPEC_DIR_ABS/scopes" -type f -name report.md | sort)
  fi
}

path_family() {
  local path="$1"
  local base="${path##*/}"

  case "$path" in
    specs|specs/*|.specify|.specify/*)
      printf 'planning'
      return 0
      ;;
  esac

  if [[ "$path" == tests/* || "$path" == */tests/* || "$path" == */__tests__/* || "$base" == test_* || "$base" == *_test.* || "$base" == *-selftest.sh ]]; then
    printf 'test'
  elif [[ "$path" == docs/* || "$path" == agents/* || "$path" == README.md || "$path" == CHANGELOG.md || "$base" == *.md ]]; then
    printf 'docs'
  elif [[ "$path" == proto/* || "$base" == *.proto || "$base" == *openapi* || "$base" == *contract* || "$path" == */contract.yaml || "$path" == */contract.yml ]]; then
    printf 'contract'
  elif [[ "$path" == config/* || "$path" == .github/workflows/* || "$path" == bubbles/workflows.yaml || "$base" == docker-compose*.yml || "$base" == docker-compose*.yaml || "$base" == Dockerfile* || "$base" == *.toml || "$base" == *.yaml || "$base" == *.yml || "$base" == *.json || "$base" == *.env ]]; then
    printf 'config'
  elif [[ "$path" == bubbles/scripts/* || "$path" == scripts/* || "$path" == bin/* || "$path" == deploy/* || "$base" == *.sh || "$base" == Makefile ]]; then
    printf 'runtime'
  elif [[ "$base" == *.rs || "$base" == *.go || "$base" == *.py || "$base" == *.ts || "$base" == *.tsx || "$base" == *.js || "$base" == *.jsx || "$base" == *.dart || "$base" == *.java || "$base" == *.scala ]]; then
    printf 'source'
  else
    printf 'other'
  fi
}

planning_paths=()
source_paths=()
runtime_paths=()
config_paths=()
contract_paths=()
test_paths=()
docs_paths=()
other_paths=()

classify_paths() {
  local path family
  for path in "${changed_paths[@]}"; do
    family="$(path_family "$path")"
    case "$family" in
      planning) planning_paths+=("$path") ;;
      source) source_paths+=("$path") ;;
      runtime) runtime_paths+=("$path") ;;
      config) config_paths+=("$path") ;;
      contract) contract_paths+=("$path") ;;
      test) test_paths+=("$path") ;;
      docs) docs_paths+=("$path") ;;
      *) other_paths+=("$path") ;;
    esac
  done
}

emit_group() {
  local stream="$1"
  local label="$2"
  shift 2
  local paths=("$@")
  local path

  if [[ "$stream" == "stderr" ]]; then
    echo "  $label (${#paths[@]}):" >&2
    for path in "${paths[@]}"; do
      echo "    - $path" >&2
    done
  else
    echo "  $label (${#paths[@]}):"
    for path in "${paths[@]}"; do
      echo "    - $path"
    done
  fi
}

emit_classification() {
  local stream="$1"
  if [[ "$stream" == "stderr" ]]; then
    echo "changedPathClassification:" >&2
    echo "  evidenceSources: git=$git_path_events report=$report_path_events reportCodeDiffSections=$report_code_diff_sections" >&2
  else
    echo "changedPathClassification:"
    echo "  evidenceSources: git=$git_path_events report=$report_path_events reportCodeDiffSections=$report_code_diff_sections"
  fi

  emit_group "$stream" "planning-only" "${planning_paths[@]}"
  emit_group "$stream" "source" "${source_paths[@]}"
  emit_group "$stream" "runtime" "${runtime_paths[@]}"
  emit_group "$stream" "config" "${config_paths[@]}"
  emit_group "$stream" "contract" "${contract_paths[@]}"
  emit_group "$stream" "test" "${test_paths[@]}"
  emit_group "$stream" "docs" "${docs_paths[@]}"
  emit_group "$stream" "other" "${other_paths[@]}"
}

collect_git_paths
collect_report_paths
classify_paths

delivery_delta_count=$((${#source_paths[@]} + ${#runtime_paths[@]} + ${#config_paths[@]} + ${#contract_paths[@]} + ${#test_paths[@]} + ${#docs_paths[@]}))
planning_only_count=${#planning_paths[@]}
other_count=${#other_paths[@]}

if [[ "$status_ceiling" != "done" ]]; then
  if [[ "$QUIET" != "true" ]]; then
    echo "delivery-implementation-delta-guard: SKIP Gate G093 (delivery_implementation_delta_gate) - workflowMode=$state_workflow_mode statusCeiling=$status_ceiling stateStatus=${state_status:-<empty>}"
    echo "lower ceiling prevents done certification; G093 delivery delta enforcement is not applicable"
    if [[ "$status_ceiling" == "specs_hardened" || "$planning_only" == "true" ]]; then
      echo "G087 remains responsible for planning packet linkage (planningOnly=$planning_only justificationPresent=$([[ -n "$planning_only_justification" ]] && echo true || echo false))"
    fi
  fi
  exit 0
fi

if [[ "$delivery_delta_count" -gt 0 ]]; then
  if [[ "$QUIET" != "true" ]]; then
    echo "delivery-implementation-delta-guard: PASS Gate G093 (delivery_implementation_delta_gate) - workflowMode=$state_workflow_mode statusCeiling=$status_ceiling deliveryDeltaPaths=$delivery_delta_count planningOnlyPaths=$planning_only_count otherPaths=$other_count"
    echo "G053-compatible evidence source accepted: gitPathEvents=$git_path_events reportPathEvents=$report_path_events reportCodeDiffSections=$report_code_diff_sections"
    emit_classification stdout
  fi
  exit 0
fi

echo "G093 delivery_implementation_delta_gate violation: done-ceiling delivery mode '$state_workflow_mode' has no implementation/runtime/config/contract/test/docs delta outside specs/ and .specify/" >&2
echo "spec=$spec_rel statusCeiling=$status_ceiling stateStatus=${state_status:-<empty>} deliveryDeltaPaths=$delivery_delta_count planningOnlyPaths=$planning_only_count otherPaths=$other_count" >&2
emit_classification stderr
echo "nextOwner: implementation" >&2
echo "alternateOwner: planning-only downgrade (use a below-done planning workflow when the packet intentionally changes only specs/ or .specify/)" >&2
echo "RESULT-ENVELOPE:" >&2
echo "  outcome: blocked" >&2
echo "  unresolvedFindings:" >&2
echo "    - finding: G093" >&2
echo "      gate: delivery_implementation_delta_gate" >&2
echo "      owner: bubbles.implement" >&2
echo "      alternateOwner: bubbles.plan (planning-only downgrade)" >&2
echo "      reason: done-ceiling delivery requires non-planning delta outside specs/ and .specify/" >&2
echo "      evidenceSources: git=$git_path_events report=$report_path_events reportCodeDiffSections=$report_code_diff_sections" >&2
exit 1