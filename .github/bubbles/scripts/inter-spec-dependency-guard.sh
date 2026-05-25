#!/usr/bin/env bash
set -euo pipefail

# inter-spec-dependency-guard.sh
#
# Gate G089 - inter_spec_dependency_gate.
#
# Specs may declare explicit inter-spec dependencies in state.json
# specDependsOn[]. Every dependency must resolve to a real spec directory
# containing state.json. Stable dependency status is done. Legacy
# done_with_concerns is read-only compatibility only for old untouched specs;
# new or recertified done_with_concerns is rejected by the G092 boundary.
# Unstable dependency statuses are accepted only when the current spec is
# explicitly marked requiresRevalidation:true, which is the durable marker
# that the dependent spec must be reviewed again.
#
# Usage:
#   bash bubbles/scripts/inter-spec-dependency-guard.sh <specDir> [--quiet] [--repo-root <root>]
#
# Exit codes:
#   0  clean
#   1  one or more G089 dependency violations
#   2  runtime error, invalid arguments, missing current state, or malformed JSON

QUIET="false"
SPEC_DIR_INPUT=""
REPO_ROOT_INPUT=""

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/inter-spec-dependency-guard.sh <specDir> [--quiet] [--repo-root <root>]

Arguments:
  <specDir>          Spec directory containing state.json.

Optional:
  --quiet            Suppress success output.
  --repo-root <root> Repository root used to resolve repo-relative dependency paths.
  -h, --help         Print this usage and exit.

Exit codes:
  0 = clean
  1 = G089 inter-spec dependency violation
  2 = runtime error, invalid arguments, missing current state, or malformed JSON
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
    --repo-root)
      [[ $# -ge 2 ]] || { echo "inter-spec-dependency-guard: --repo-root requires a value" >&2; exit 2; }
      REPO_ROOT_INPUT="$2"
      shift 2
      ;;
    --*)
      echo "inter-spec-dependency-guard: unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -n "$SPEC_DIR_INPUT" ]]; then
        echo "inter-spec-dependency-guard: only one specDir may be supplied" >&2
        usage >&2
        exit 2
      fi
      SPEC_DIR_INPUT="$1"
      shift
      ;;
  esac
done

if [[ -z "$SPEC_DIR_INPUT" ]]; then
  echo "inter-spec-dependency-guard: missing required specDir" >&2
  usage >&2
  exit 2
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "inter-spec-dependency-guard: jq is required but not found in PATH" >&2
  exit 2
fi

if [[ ! -d "$SPEC_DIR_INPUT" ]]; then
  echo "inter-spec-dependency-guard: specDir does not exist: $SPEC_DIR_INPUT" >&2
  exit 2
fi

SPEC_DIR_ABS="$(cd "$SPEC_DIR_INPUT" && pwd -P)"
STATE_FILE="$SPEC_DIR_ABS/state.json"

if [[ ! -f "$STATE_FILE" ]]; then
  echo "inter-spec-dependency-guard: state.json not found: $STATE_FILE" >&2
  exit 2
fi

if ! jq -e 'type == "object"' "$STATE_FILE" >/dev/null 2>&1; then
  echo "inter-spec-dependency-guard: malformed or non-object JSON: $STATE_FILE" >&2
  exit 2
fi

resolve_repo_root() {
  if [[ -n "$REPO_ROOT_INPUT" ]]; then
    (cd "$REPO_ROOT_INPUT" && pwd -P)
    return 0
  fi

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
    if [[ -d "$dir/specs" && ( -f "$dir/bubbles/workflows.yaml" || -d "$dir/.specify/memory" ) ]]; then
      printf '%s\n' "$dir"
      return 0
    fi
    dir="$(dirname "$dir")"
  done

  return 1
}

REPO_ROOT="$(resolve_repo_root || true)"
if [[ -z "$REPO_ROOT" || ! -d "$REPO_ROOT" ]]; then
  echo "inter-spec-dependency-guard: unable to resolve repository root from specDir: $SPEC_DIR_INPUT" >&2
  exit 2
fi

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

resolve_link_path() {
  local link="$1"
  if [[ "$link" == /* ]]; then
    printf '%s' "$link"
  else
    printf '%s/%s' "$REPO_ROOT" "${link#./}"
  fi
}

canonical_dir_or_empty() {
  local path="$1"
  if [[ -d "$path" ]]; then
    (cd "$path" && pwd -P)
  fi
}

is_stable_status() {
  case "$1" in
    done) return 0 ;;
    *) return 1 ;;
  esac
}

is_legacy_done_with_concerns_state() {
  local file="$1"
  jq -e '
    (.status == "done_with_concerns")
    and ((.legacyStatusCompatibility == true) or (.certification.legacyStatusCompatibility == true))
    and ((.certifiedAt? // "") | type == "string" and length > 0)
    and (.requiresRevalidation != true)
    and (.touchedForRecertification != true)
    and (.recertifying != true)
    and (.certification.touched != true)
    and (.certification.recertifying != true)
    and ((.proposedStatus? // "") != "done_with_concerns")
    and ((.certification.proposedStatus? // "") != "done_with_concerns")
    and ((.certification.targetStatus? // "") != "done_with_concerns")
    and ([.transitionRequests[]? | select(((.status? // .toStatus? // .targetStatus? // .outcome? // .proposedStatus? // "") == "done_with_concerns"))] | length == 0)
  ' "$file" >/dev/null
}

json_string_field_or_empty() {
  local expr="$1"
  local file="$2"
  jq -r "$expr" "$file"
}

validate_state_shape() {
  local file="$1"
  local label="$2"

  if ! jq -e 'type == "object"' "$file" >/dev/null 2>&1; then
    echo "inter-spec-dependency-guard: malformed or non-object JSON for $label: $file" >&2
    exit 2
  fi

  local status_type
  status_type="$(jq -r 'if has("status") then (.status | type) else "missing" end' "$file")"
  if [[ "$status_type" != "string" && "$status_type" != "missing" ]]; then
    echo "inter-spec-dependency-guard: state.status must be a string when present for $label: $file" >&2
    exit 2
  fi

  local dep_type
  dep_type="$(jq -r 'if has("specDependsOn") then (.specDependsOn | type) else "array" end' "$file")"
  if [[ "$dep_type" != "array" ]]; then
    echo "inter-spec-dependency-guard: specDependsOn must be an array when present for $label: $file" >&2
    exit 2
  fi

  if ! jq -e '([.specDependsOn[]? | select(type != "string")] | length) == 0' "$file" >/dev/null; then
    echo "inter-spec-dependency-guard: specDependsOn entries must be strings for $label: $file" >&2
    exit 2
  fi

  local revalidation_type
  revalidation_type="$(jq -r 'if has("requiresRevalidation") then (.requiresRevalidation | type) else "boolean" end' "$file")"
  if [[ "$revalidation_type" != "boolean" ]]; then
    echo "inter-spec-dependency-guard: requiresRevalidation must be boolean when present for $label: $file" >&2
    exit 2
  fi
}

format_trail() {
  local trail="$1"
  local trimmed="${trail#|}"
  trimmed="${trimmed%|}"
  local IFS='|'
  local part
  local output=""
  for part in $trimmed; do
    [[ -n "$part" ]] || continue
    if [[ -z "$output" ]]; then
      output="$(relative_path "$part")"
    else
      output="$output -> $(relative_path "$part")"
    fi
  done
  printf '%s' "$output"
}

finding_count=0
acknowledged_unstable_count=0
dependency_count=0
accepted_dependencies=()
VISITED="|"

violation() {
  echo "G089 inter_spec_dependency_gate violation: $*" >&2
  finding_count=$((finding_count + 1))
}

validate_state_shape "$STATE_FILE" "current spec"

spec_rel="$(relative_path "$SPEC_DIR_ABS")"
current_status="$(json_string_field_or_empty '.status // ""' "$STATE_FILE")"
requires_revalidation="$(json_string_field_or_empty 'if .requiresRevalidation == true then "true" else "false" end' "$STATE_FILE")"

if [[ "$requires_revalidation" == "true" && ( "$current_status" == "done" || "$current_status" == "done_with_concerns" ) ]]; then
  violation "$spec_rel has status '$current_status' while requiresRevalidation:true is unresolved; demote the spec or recertify after revalidation"
fi

visit_spec() {
  local spec_abs="$1"
  local trail="$2"
  local spec_state="$spec_abs/state.json"
  local spec_label
  spec_label="$(relative_path "$spec_abs")"

  if [[ ! -f "$spec_state" ]]; then
    violation "$spec_label is missing state.json"
    return 0
  fi

  if [[ "$trail" == *"|$spec_abs|"* ]]; then
    violation "dependency cycle detected: $(format_trail "${trail}${spec_abs}|")"
    return 0
  fi

  if [[ "$VISITED" == *"|$spec_abs|"* ]]; then
    return 0
  fi

  validate_state_shape "$spec_state" "$spec_label"

  local next_trail
  next_trail="${trail}${spec_abs}|"

  while IFS= read -r dependency; do
    [[ -n "$dependency" ]] || continue
    if [[ "$spec_abs" == "$SPEC_DIR_ABS" ]]; then
      dependency_count=$((dependency_count + 1))
    fi

    local dependency_path dependency_abs dependency_rel dependency_state dependency_status dependency_status_type
    dependency_path="$(resolve_link_path "$dependency")"
    dependency_rel="$(relative_path "$dependency_path")"
    dependency_abs="$(canonical_dir_or_empty "$dependency_path")"

    if [[ -z "$dependency_abs" ]]; then
      violation "$spec_label specDependsOn entry is dangling: $dependency (resolved: $dependency_rel)"
      continue
    fi

    dependency_state="$dependency_abs/state.json"
    if [[ ! -f "$dependency_state" ]]; then
      violation "$spec_label specDependsOn entry has no state.json: $dependency (resolved: $(relative_path "$dependency_abs"))"
      continue
    fi

    validate_state_shape "$dependency_state" "$(relative_path "$dependency_abs")"
    dependency_status_type="$(jq -r 'if has("status") then (.status | type) else "missing" end' "$dependency_state")"
    if [[ "$dependency_status_type" != "string" ]]; then
      echo "inter-spec-dependency-guard: dependency state.status is required and must be a string for $(relative_path "$dependency_abs")" >&2
      exit 2
    fi
    dependency_status="$(jq -r '.status' "$dependency_state")"

    if is_stable_status "$dependency_status"; then
      if [[ "$spec_abs" == "$SPEC_DIR_ABS" ]]; then
        accepted_dependencies+=("$(relative_path "$dependency_abs"):$dependency_status")
      fi
    elif [[ "$dependency_status" == "done_with_concerns" ]] && is_legacy_done_with_concerns_state "$dependency_state"; then
      if [[ "$spec_abs" == "$SPEC_DIR_ABS" ]]; then
        accepted_dependencies+=("$(relative_path "$dependency_abs"):$dependency_status(legacy-read-only)")
      fi
    elif [[ "$dependency_status" == "done_with_concerns" ]]; then
      violation "$spec_label depends on $(relative_path "$dependency_abs") with new or recertified dependency status 'done_with_concerns' — G092 compatibility boundary allows only legacy read-only done_with_concerns; stable new dependency status must be done"
    elif [[ "$requires_revalidation" == "true" ]]; then
      acknowledged_unstable_count=$((acknowledged_unstable_count + 1))
    else
      violation "$spec_label depends on $(relative_path "$dependency_abs") with invalid dependency status '$dependency_status' (allowed: done; legacy read-only done_with_concerns only until touched or recertified)"
    fi

    visit_spec "$dependency_abs" "$next_trail"
  done < <(jq -r '.specDependsOn[]?' "$spec_state")

  VISITED="${VISITED}${spec_abs}|"
}

visit_spec "$SPEC_DIR_ABS" "|"

if [[ "$finding_count" -gt 0 ]]; then
  echo "G089 inter_spec_dependency_gate blocked: findings=$finding_count spec=$spec_rel dependencies=$dependency_count requiresRevalidation=$requires_revalidation" >&2
  exit 1
fi

if [[ "$QUIET" != "true" ]]; then
  if [[ "${#accepted_dependencies[@]}" -gt 0 ]]; then
    accepted_joined="${accepted_dependencies[*]}"
  else
    accepted_joined="none"
  fi
  echo "inter-spec-dependency-guard: PASS Gate G089 (inter_spec_dependency_gate) - spec=$spec_rel dependencies=$dependency_count acceptedDependencies=$accepted_joined requiresRevalidation=$requires_revalidation acknowledgedUnstableDependencies=$acknowledged_unstable_count"
fi

exit 0