#!/usr/bin/env bash
set -euo pipefail

# inter-spec-dependency-revalidation.sh
#
# Helper for Gate G089. When a dependency spec is demoted or otherwise not
# stable, mark every direct or transitive dependent spec with
# requiresRevalidation:true. The mutation is additive: it only writes the
# revalidation flag and preserves all other state fields.

QUIET="false"
DEPENDENCY_INPUT=""
REPO_ROOT_INPUT=""

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/inter-spec-dependency-revalidation.sh <dependencySpecDir> [--quiet] [--repo-root <root>]

Arguments:
  <dependencySpecDir> Dependency spec directory whose current state should be checked.

Optional:
  --quiet            Suppress success output.
  --repo-root <root> Repository root used to resolve repo-relative dependency paths.
  -h, --help         Print this usage and exit.

Exit codes:
  0 = helper completed; dependents are marked when dependency is unstable
  2 = runtime error, invalid arguments, missing dependency state, or malformed JSON
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
      [[ $# -ge 2 ]] || { echo "inter-spec-dependency-revalidation: --repo-root requires a value" >&2; exit 2; }
      REPO_ROOT_INPUT="$2"
      shift 2
      ;;
    --*)
      echo "inter-spec-dependency-revalidation: unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -n "$DEPENDENCY_INPUT" ]]; then
        echo "inter-spec-dependency-revalidation: only one dependency spec may be supplied" >&2
        usage >&2
        exit 2
      fi
      DEPENDENCY_INPUT="$1"
      shift
      ;;
  esac
done

if [[ -z "$DEPENDENCY_INPUT" ]]; then
  echo "inter-spec-dependency-revalidation: missing dependency spec directory" >&2
  usage >&2
  exit 2
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "inter-spec-dependency-revalidation: jq is required but not found in PATH" >&2
  exit 2
fi

if [[ ! -d "$DEPENDENCY_INPUT" ]]; then
  echo "inter-spec-dependency-revalidation: dependency specDir does not exist: $DEPENDENCY_INPUT" >&2
  exit 2
fi

DEPENDENCY_ABS="$(cd "$DEPENDENCY_INPUT" && pwd -P)"
DEPENDENCY_STATE="$DEPENDENCY_ABS/state.json"

if [[ ! -f "$DEPENDENCY_STATE" ]]; then
  echo "inter-spec-dependency-revalidation: dependency state.json not found: $DEPENDENCY_STATE" >&2
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
  parent="$(dirname "$DEPENDENCY_ABS")"
  if [[ "$(basename "$parent")" == "specs" ]]; then
    (cd "$(dirname "$parent")" && pwd -P)
    return 0
  fi

  local dir
  dir="$DEPENDENCY_ABS"
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
  echo "inter-spec-dependency-revalidation: unable to resolve repository root from dependency: $DEPENDENCY_INPUT" >&2
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
  local status="$1"
  local state_file="${2:-}"
  case "$status" in
    done) return 0 ;;
    done_with_concerns)
      [[ -n "$state_file" ]] || return 1
      jq -e '(.legacyStatusCompatibility == true) or (.certification.legacyStatusCompatibility == true)' "$state_file" >/dev/null 2>&1
      return $?
      ;;
    *) return 1 ;;
  esac
}

validate_state_shape() {
  local file="$1"
  local label="$2"
  if ! jq -e 'type == "object"' "$file" >/dev/null 2>&1; then
    echo "inter-spec-dependency-revalidation: malformed or non-object JSON for $label: $file" >&2
    exit 2
  fi
  if ! jq -e 'if has("specDependsOn") then ((.specDependsOn | type) == "array" and ([.specDependsOn[]? | select(type != "string")] | length) == 0) else true end' "$file" >/dev/null; then
    echo "inter-spec-dependency-revalidation: specDependsOn must be an array of strings for $label: $file" >&2
    exit 2
  fi
  if ! jq -e 'if has("requiresRevalidation") then ((.requiresRevalidation | type) == "boolean") else true end' "$file" >/dev/null; then
    echo "inter-spec-dependency-revalidation: requiresRevalidation must be boolean when present for $label: $file" >&2
    exit 2
  fi
}

mark_revalidation() {
  local state_file="$1"
  local tmp_file
  tmp_file="$(mktemp)"
  jq '.requiresRevalidation = true' "$state_file" > "$tmp_file"
  mv "$tmp_file" "$state_file"
}

validate_state_shape "$DEPENDENCY_STATE" "dependency $(relative_path "$DEPENDENCY_ABS")"
dependency_status_type="$(jq -r 'if has("status") then (.status | type) else "missing" end' "$DEPENDENCY_STATE")"
if [[ "$dependency_status_type" != "string" ]]; then
  echo "inter-spec-dependency-revalidation: dependency state.status is required and must be a string for $(relative_path "$DEPENDENCY_ABS")" >&2
  exit 2
fi
dependency_status="$(jq -r '.status' "$DEPENDENCY_STATE")"

affected="|$DEPENDENCY_ABS|"
marked_count=0

if ! is_stable_status "$dependency_status" "$DEPENDENCY_STATE"; then
  changed="true"
  while [[ "$changed" == "true" ]]; do
    changed="false"
    while IFS= read -r state_file; do
      [[ -n "$state_file" ]] || continue
      spec_abs="$(cd "$(dirname "$state_file")" && pwd -P)"
      if [[ "$affected" == *"|$spec_abs|"* ]]; then
        continue
      fi

      validate_state_shape "$state_file" "$(relative_path "$spec_abs")"
      depends_on_affected="false"
      while IFS= read -r dependency; do
        [[ -n "$dependency" ]] || continue
        dep_path="$(resolve_link_path "$dependency")"
        dep_abs="$(canonical_dir_or_empty "$dep_path")"
        if [[ -n "$dep_abs" && "$affected" == *"|$dep_abs|"* ]]; then
          depends_on_affected="true"
          break
        fi
      done < <(jq -r '.specDependsOn[]?' "$state_file")

      if [[ "$depends_on_affected" == "true" ]]; then
        current_flag="$(jq -r 'if .requiresRevalidation == true then "true" else "false" end' "$state_file")"
        if [[ "$current_flag" != "true" ]]; then
          mark_revalidation "$state_file"
          marked_count=$((marked_count + 1))
          changed="true"
        fi
        affected="${affected}${spec_abs}|"
      fi
    done < <(find "$REPO_ROOT/specs" -mindepth 2 -maxdepth 2 -type f -name 'state.json' | sort)
  done
fi

if [[ "$QUIET" != "true" ]]; then
  echo "inter-spec-dependency-revalidation: PASS Gate G089 helper - dependency=$(relative_path "$DEPENDENCY_ABS") dependencyStatus=$dependency_status markedDependents=$marked_count"
fi

exit 0