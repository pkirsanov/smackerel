#!/usr/bin/env bash
set -euo pipefail

# planning-packet-linkage-guard.sh
#
# Gate G087 - planning_packet_implementation_linkage_gate.
#
# Hardened planning packets must either link to the implementation spec
# that consumes them or explicitly declare themselves planning-only with
# a non-empty justification. Done implementation specs must point back
# to the hardened planning packet that fed them.
#
# Usage:
#   bash bubbles/scripts/planning-packet-linkage-guard.sh <specDir> [--quiet]
#
# Exit codes:
#   0  clean
#   1  one or more G087 linkage violations
#   2  runtime error, invalid arguments, missing state, malformed JSON

QUIET="false"
SPEC_DIR_INPUT=""

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/planning-packet-linkage-guard.sh <specDir> [--quiet]

Arguments:
  <specDir>  Spec directory containing state.json.

Optional:
  --quiet    Suppress success output.
  -h, --help Print this usage and exit.

Exit codes:
  0 = clean
  1 = G087 planning packet linkage violation
  2 = runtime error, invalid arguments, missing state, malformed JSON
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
    --*)
      echo "planning-packet-linkage-guard: unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -n "$SPEC_DIR_INPUT" ]]; then
        echo "planning-packet-linkage-guard: only one specDir may be supplied" >&2
        usage >&2
        exit 2
      fi
      SPEC_DIR_INPUT="$1"
      shift
      ;;
  esac
done

if [[ -z "$SPEC_DIR_INPUT" ]]; then
  echo "planning-packet-linkage-guard: missing required specDir" >&2
  usage >&2
  exit 2
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "planning-packet-linkage-guard: jq is required but not found in PATH" >&2
  exit 2
fi

if [[ ! -d "$SPEC_DIR_INPUT" ]]; then
  echo "planning-packet-linkage-guard: specDir does not exist: $SPEC_DIR_INPUT" >&2
  exit 2
fi

SPEC_DIR_ABS="$(cd "$SPEC_DIR_INPUT" && pwd -P)"
STATE_FILE="$SPEC_DIR_ABS/state.json"

if [[ ! -f "$STATE_FILE" ]]; then
  echo "planning-packet-linkage-guard: state.json not found: $STATE_FILE" >&2
  exit 2
fi

if ! jq -e 'type == "object"' "$STATE_FILE" >/dev/null 2>&1; then
  echo "planning-packet-linkage-guard: malformed or non-object JSON: $STATE_FILE" >&2
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
  echo "planning-packet-linkage-guard: unable to resolve repository root from specDir: $SPEC_DIR_INPUT" >&2
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

state_string_or_empty() {
  local expr="$1"
  local file="$2"
  jq -r "$expr" "$file"
}

finding_count=0
spec_rel="$(relative_path "$SPEC_DIR_ABS")"

violation() {
  echo "G087 planning_packet_implementation_linkage_gate violation: $*" >&2
  finding_count=$((finding_count + 1))
}

status="$(state_string_or_empty '.status // ""' "$STATE_FILE")"
planning_only="$(state_string_or_empty 'if .planningOnly == true then "true" else "false" end' "$STATE_FILE")"
planning_only_justification="$(state_string_or_empty '.planningOnlyJustification // ""' "$STATE_FILE")"
link_type="$(state_string_or_empty '(.linkedImplementationSpec | type) // "null"' "$STATE_FILE")"
linked_implementation="$(state_string_or_empty '.linkedImplementationSpec // ""' "$STATE_FILE")"

if [[ "$planning_only" == "true" && "$planning_only_justification" =~ ^[[:space:]]*$ ]]; then
  violation "$spec_rel sets planningOnly:true but planningOnlyJustification is empty or null"
fi

if [[ "$status" == "specs_hardened" && "$planning_only" != "true" ]]; then
  if [[ "$link_type" != "string" || "$linked_implementation" =~ ^[[:space:]]*$ ]]; then
    violation "$spec_rel has status specs_hardened and planningOnly is not true, but linkedImplementationSpec is missing or empty"
  else
    impl_path="$(resolve_link_path "$linked_implementation")"
    impl_rel="$(relative_path "$impl_path")"
    if [[ ! -d "$impl_path" || ! -f "$impl_path/state.json" ]]; then
      violation "$spec_rel linkedImplementationSpec is dangling: $linked_implementation (resolved: $impl_rel)"
    else
      impl_abs="$(canonical_dir_or_empty "$impl_path")"
      impl_state="$impl_abs/state.json"
      if ! jq -e 'type == "object"' "$impl_state" >/dev/null 2>&1; then
        echo "planning-packet-linkage-guard: malformed or non-object linked state.json: $impl_state" >&2
        exit 2
      fi

      impl_status="$(state_string_or_empty '.status // ""' "$impl_state")"
      if [[ "$impl_status" == "archived" ]]; then
        violation "$spec_rel linkedImplementationSpec points to archived implementation target $impl_rel; relink to an active implementation spec or classify the packet as planningOnly:true with planningOnlyJustification"
      elif [[ "$impl_status" == "done" ]]; then
        back_link_type="$(state_string_or_empty '(.linkedPlanningPacket | type) // "null"' "$impl_state")"
        back_link="$(state_string_or_empty '.linkedPlanningPacket // ""' "$impl_state")"
        if [[ "$back_link_type" != "string" || "$back_link" =~ ^[[:space:]]*$ ]]; then
          violation "$impl_rel has status done but linkedPlanningPacket does not point back to $spec_rel"
        else
          back_path="$(resolve_link_path "$back_link")"
          back_abs="$(canonical_dir_or_empty "$back_path")"
          if [[ -z "$back_abs" || "$back_abs" != "$SPEC_DIR_ABS" ]]; then
            violation "$impl_rel has status done but linkedPlanningPacket='$back_link' does not point back to $spec_rel"
          fi
        fi
      fi
    fi
  fi
fi

if [[ "$finding_count" -gt 0 ]]; then
  echo "G087 planning_packet_implementation_linkage_gate blocked: findings=$finding_count spec=$spec_rel" >&2
  exit 1
fi

if [[ "$QUIET" != "true" ]]; then
  echo "planning-packet-linkage-guard: PASS Gate G087 (planning_packet_implementation_linkage_gate) - spec=$spec_rel status=${status:-<empty>} planningOnly=$planning_only"
fi

exit 0