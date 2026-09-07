#!/usr/bin/env bash
# Assign existing release-train metadata to one state.json without lifecycle effects.

set -euo pipefail
umask 077

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPOSITORY_BINDING="$SCRIPT_DIR/repository-binding.sh"
RUNNER_GRANTS="$SCRIPT_DIR/workflow-runner-grants-lint.sh"
ASSIGNMENT_MODE="release-train-assign-metadata"

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/release-train-metadata-assign.sh <spec-dir|state.json> --train <train-id> [options] \
  --session-id <id> --session-control-file <external-path> \
  --binding-packet-file <packet.json> --workflow-mode release-train-assign-metadata

Options:
  --flags-json <json-array>  Set flagsIntroduced to unique non-empty strings.
  --dry-run                  Print the candidate JSON without writing (default).
  --apply                    Atomically replace state.json when values change.
  --session-id <id>          Session id carried by the repository binding.
  --session-control-file <path>
                             External authoritative session control record.
  --binding-packet-file <path>
                             Current local actionable repository packet.
  --workflow-mode <mode>     Must be release-train-assign-metadata.
  --runner <agent>           Runtime-supplied active top-level runner identity.
  -h, --help                 Print this help text.

Every invocation validates repository authority, runner admission, artifact
ownership, and mode-owned fields. Apply also requires the active top-level
runner to be bubbles.train. BUBBLES_AGENT_NAME is never used as authority. The command
changes only releaseTrain and an explicitly supplied flagsIntroduced value.
EOF
}

refuse() {
  echo "release-train-metadata-assign: REFUSED: $*" >&2
  exit 2
}

TARGET=""
TRAIN=""
FLAGS_JSON=""
FLAGS_SUPPLIED=0
TRAIN_SUPPLIED=0
MODE="dry-run"
MODE_SUPPLIED=0
SESSION_ID=""
SESSION_CONTROL_FILE=""
BINDING_PACKET_FILE=""
WORKFLOW_MODE=""
RUNNER=""
SESSION_ID_SUPPLIED=0
SESSION_CONTROL_SUPPLIED=0
BINDING_PACKET_SUPPLIED=0
WORKFLOW_MODE_SUPPLIED=0
RUNNER_SUPPLIED=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --train)
      [[ "$TRAIN_SUPPLIED" -eq 0 ]] || refuse "--train must be supplied exactly once"
      [[ $# -ge 2 ]] || refuse "--train requires a value"
      TRAIN="$2"
      TRAIN_SUPPLIED=1
      shift 2
      ;;
    --flags-json)
      [[ "$FLAGS_SUPPLIED" -eq 0 ]] || refuse "--flags-json may be supplied at most once"
      [[ $# -ge 2 ]] || refuse "--flags-json requires a value"
      FLAGS_JSON="$2"
      FLAGS_SUPPLIED=1
      shift 2
      ;;
    --session-id)
      [[ "$SESSION_ID_SUPPLIED" -eq 0 ]] || refuse "--session-id may be supplied only once"
      [[ $# -ge 2 ]] || refuse "--session-id requires a value"
      SESSION_ID="$2"
      SESSION_ID_SUPPLIED=1
      shift 2
      ;;
    --session-control-file)
      [[ "$SESSION_CONTROL_SUPPLIED" -eq 0 ]] || refuse "--session-control-file may be supplied only once"
      [[ $# -ge 2 ]] || refuse "--session-control-file requires a value"
      SESSION_CONTROL_FILE="$2"
      SESSION_CONTROL_SUPPLIED=1
      shift 2
      ;;
    --binding-packet-file)
      [[ "$BINDING_PACKET_SUPPLIED" -eq 0 ]] || refuse "--binding-packet-file may be supplied only once"
      [[ $# -ge 2 ]] || refuse "--binding-packet-file requires a value"
      BINDING_PACKET_FILE="$2"
      BINDING_PACKET_SUPPLIED=1
      shift 2
      ;;
    --workflow-mode)
      [[ "$WORKFLOW_MODE_SUPPLIED" -eq 0 ]] || refuse "--workflow-mode may be supplied only once"
      [[ $# -ge 2 ]] || refuse "--workflow-mode requires a value"
      WORKFLOW_MODE="$2"
      WORKFLOW_MODE_SUPPLIED=1
      shift 2
      ;;
    --runner)
      [[ "$RUNNER_SUPPLIED" -eq 0 ]] || refuse "--runner may be supplied only once"
      [[ $# -ge 2 ]] || refuse "--runner requires a value"
      RUNNER="$2"
      RUNNER_SUPPLIED=1
      shift 2
      ;;
    --dry-run|--apply)
      [[ "$MODE_SUPPLIED" -eq 0 ]] || refuse "--dry-run and --apply are mutually exclusive and may be supplied only once"
      MODE="${1#--}"
      MODE_SUPPLIED=1
      shift
      ;;
    --*)
      refuse "unknown option: $1"
      ;;
    *)
      [[ -z "$TARGET" ]] || refuse "only one target may be supplied"
      TARGET="$1"
      shift
      ;;
  esac
done

[[ -n "$TARGET" ]] || refuse "one target spec directory or state.json is required"
[[ "$TRAIN_SUPPLIED" -eq 1 && -n "$TRAIN" ]] || refuse "--train must be supplied exactly once with a non-empty value"
[[ "$SESSION_ID_SUPPLIED" -eq 1 && -n "$SESSION_ID" ]] || refuse "--session-id is required"
[[ "$SESSION_CONTROL_SUPPLIED" -eq 1 && -n "$SESSION_CONTROL_FILE" ]] || refuse "--session-control-file is required"
[[ "$BINDING_PACKET_SUPPLIED" -eq 1 && -n "$BINDING_PACKET_FILE" ]] || refuse "--binding-packet-file is required"
[[ "$WORKFLOW_MODE_SUPPLIED" -eq 1 && "$WORKFLOW_MODE" == "$ASSIGNMENT_MODE" ]] || refuse "workflow mode must be $ASSIGNMENT_MODE"
[[ "$RUNNER_SUPPLIED" -eq 1 && -n "$RUNNER" ]] || refuse "--runner is required from the authenticated active top-level runtime"

command -v jq >/dev/null 2>&1 || refuse "jq is required"
command -v yq >/dev/null 2>&1 || refuse "yq is required"
[[ -f "$REPOSITORY_BINDING" ]] || refuse "repository binding validator is unavailable"
[[ -f "$RUNNER_GRANTS" ]] || refuse "workflow runner admission evaluator is unavailable"

BINDING_OUTPUT=""
if ! BINDING_OUTPUT="$(bash "$REPOSITORY_BINDING" validate-packet \
  --session-id "$SESSION_ID" \
  --session-control-file "$SESSION_CONTROL_FILE" \
  --packet-file "$BINDING_PACKET_FILE")"; then
  [[ -z "$BINDING_OUTPUT" ]] || printf '%s\n' "$BINDING_OUTPUT" >&2
  refuse "repository binding validation failed"
fi
[[ -z "$BINDING_OUTPUT" ]] || printf '%s\n' "$BINDING_OUTPUT" >&2

REPOSITORY_ROOT="$(jq -r '.repositoryRoot' "$BINDING_PACKET_FILE")" || refuse "validated repository root is unreadable"
[[ -n "$REPOSITORY_ROOT" && "$REPOSITORY_ROOT" == /* ]] || refuse "validated repository root is invalid"

CAPABILITIES_FILE="$REPOSITORY_ROOT/bubbles/agent-capabilities.yaml"
MODES_FILE="$REPOSITORY_ROOT/bubbles/workflows/modes.yaml"
OWNERSHIP_FILE="$REPOSITORY_ROOT/bubbles/agent-ownership.yaml"
if [[ ! -f "$CAPABILITIES_FILE" ]]; then
  CAPABILITIES_FILE="$REPOSITORY_ROOT/.github/bubbles/agent-capabilities.yaml"
  MODES_FILE="$REPOSITORY_ROOT/.github/bubbles/workflows/modes.yaml"
  OWNERSHIP_FILE="$REPOSITORY_ROOT/.github/bubbles/agent-ownership.yaml"
fi
[[ -f "$CAPABILITIES_FILE" && -f "$MODES_FILE" && -f "$OWNERSHIP_FILE" ]] || refuse "workflow or ownership authorities are missing from the validated repository"
MODES_JSON="$(yq -o=json '.' "$MODES_FILE" 2>/dev/null)" || refuse "workflow modes are invalid"
OWNERSHIP_JSON="$(yq -o=json '.' "$OWNERSHIP_FILE" 2>/dev/null)" || refuse "release-train-state ownership authority is malformed"
if ! ADMISSION_OUTPUT="$(bash "$RUNNER_GRANTS" --evaluate-runner-mode "$CAPABILITIES_FILE" "$RUNNER" "$ASSIGNMENT_MODE" 2>&1)"; then
  refuse "$ADMISSION_OUTPUT"
fi
printf '%s\n' "$ADMISSION_OUTPUT" >&2

jq -e --arg mode "$ASSIGNMENT_MODE" '
  (.modes | type) == "object"
  and (.modes[$mode] | type) == "object"
  and (.modes[$mode].constraints | type) == "object"
  and (.modes[$mode].constraints.ownedStateFields | type) == "array"
  and all(.modes[$mode].constraints.ownedStateFields[]; type == "string" and length > 0)
' <<<"$MODES_JSON" >/dev/null 2>&1 || refuse "assignment mode authority is malformed"
jq -e '
  (.artifacts | type) == "object"
  and (.artifacts."release-train-state" | type) == "object"
  and (.artifacts."release-train-state".owner | type) == "array"
  and .artifacts."release-train-state".owner == ["bubbles.train"]
' <<<"$OWNERSHIP_JSON" >/dev/null 2>&1 || refuse "bubbles.train must be the sole release-train-state owner"

REQUESTED_FIELDS_JSON='["releaseTrain"]'
if [[ "$FLAGS_SUPPLIED" -eq 1 ]]; then
  REQUESTED_FIELDS_JSON='["releaseTrain","flagsIntroduced"]'
fi
jq -e --arg mode "$ASSIGNMENT_MODE" --argjson requested "$REQUESTED_FIELDS_JSON" '
  .modes[$mode].constraints.ownedStateFields as $owned
  | all($requested[]; . as $field | $owned | index($field) != null)
' <<<"$MODES_JSON" >/dev/null 2>&1 || refuse "assignment mode ownedStateFields do not contain every requested field"

if [[ "$MODE" == "apply" && "$RUNNER" != "bubbles.train" ]]; then
  if [[ "$ADMISSION_OUTPUT" == *"wildcard grant"* ]]; then
    refuse "runner '$RUNNER' was admitted by wildcard but --apply requires the direct authenticated runner bubbles.train"
  fi
  refuse "--apply requires the direct authenticated runner bubbles.train"
fi

path_has_symlink_component() {
  local path="$1"
  local remainder
  local component
  local cursor=""

  [[ "$path" == /* ]] || return 0
  remainder="${path#/}"
  while [[ -n "$remainder" ]]; do
    if [[ "$remainder" == */* ]]; then
      component="${remainder%%/*}"
      remainder="${remainder#*/}"
    else
      component="$remainder"
      remainder=""
    fi
    [[ -n "$component" ]] || continue
    [[ "$component" != "." && "$component" != ".." ]] || return 0
    cursor="$cursor/$component"
    [[ ! -L "$cursor" ]] || return 0
    [[ -e "$cursor" ]] || return 1
  done
  return 1
}

path_is_owned_and_not_writable_by_others() {
  local path="$1"
  local permissions

  [[ -O "$path" && ! -L "$path" ]] || return 1
  permissions="$(LC_ALL=C ls -ld "$path" 2>/dev/null)" || return 1
  permissions="${permissions%% *}"
  case "$permissions" in
    ?????w*|????????w*) return 1 ;;
  esac
  return 0
}

state_target_is_safe() {
  local path="$1"
  local directory

  [[ -f "$path" && "$(basename "$path")" == "state.json" ]] || return 1
  path_has_symlink_component "$path" && return 1
  case "$path" in
    "$REPOSITORY_ROOT"/specs/*/state.json|"$REPOSITORY_ROOT"/bugs/*/state.json) ;;
    *) return 1 ;;
  esac
  path_is_owned_and_not_writable_by_others "$path" || return 1
  directory="$(dirname "$path")"
  while true; do
    path_is_owned_and_not_writable_by_others "$directory" || return 1
    [[ "$directory" == "$REPOSITORY_ROOT" ]] && break
    [[ "$directory" != "/" ]] || return 1
    directory="$(dirname "$directory")"
  done
  return 0
}

file_digest() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    return 1
  fi
}

file_identity() {
  LC_ALL=C ls -di "$1" 2>/dev/null | awk 'NR == 1 { print $1 }'
}

case "$TARGET" in
  /*) TARGET_PATH="$TARGET" ;;
  *) TARGET_PATH="$REPOSITORY_ROOT/$TARGET" ;;
esac
path_has_symlink_component "$TARGET_PATH" && refuse "target path contains a symlink or unsafe component: $TARGET_PATH"
STATE_FILE="$TARGET_PATH"
if [[ -d "$TARGET_PATH" ]]; then
  STATE_FILE="$TARGET_PATH/state.json"
elif [[ "$(basename "$TARGET_PATH")" != "state.json" ]]; then
  refuse "file target must be named state.json: $TARGET"
fi
STATE_DIR="$(cd "$(dirname "$STATE_FILE")" 2>/dev/null && pwd -P)" || refuse "state.json parent is unavailable: $STATE_FILE"
STATE_FILE="$STATE_DIR/$(basename "$STATE_FILE")"
state_target_is_safe "$STATE_FILE" || refuse "state target is outside the authorized repository, contains a symlink, or has an ownership/mode violation: $STATE_FILE"
jq -e 'type == "object"' "$STATE_FILE" >/dev/null 2>&1 || refuse "state.json must contain one JSON object: $STATE_FILE"

TRAINS_FILE="$REPOSITORY_ROOT/config/release-trains.yaml"
[[ -f "$TRAINS_FILE" ]] || refuse "config/release-trains.yaml not found in validated repository"
path_has_symlink_component "$TRAINS_FILE" && refuse "train registry path contains a symlink"
path_is_owned_and_not_writable_by_others "$TRAINS_FILE" || refuse "train registry has an ownership/mode violation"

TRAINS_JSON="$(yq -o=json '.trains' "$TRAINS_FILE" 2>/dev/null)" || refuse "invalid train registry: $TRAINS_FILE"
jq -e '
  type == "array" and length > 0
  and all(.[]; (.id | type) == "string" and (.id | length) > 0)
  and ((map(.id) | unique | length) == length)
' <<<"$TRAINS_JSON" >/dev/null 2>&1 || refuse "train registry must contain unique, non-empty string IDs: $TRAINS_FILE"

if ! jq -e --arg train "$TRAIN" 'any(.[]; .id == $train)' <<<"$TRAINS_JSON" >/dev/null 2>&1; then
  refuse "unknown train '$TRAIN' is not declared in $TRAINS_FILE"
fi

if [[ "$FLAGS_SUPPLIED" -eq 1 ]]; then
  jq -e '
    type == "array"
    and all(.[]; type == "string" and length > 0)
    and ((unique | length) == length)
  ' <<<"$FLAGS_JSON" >/dev/null 2>&1 || refuse "--flags-json must be a JSON array of unique, non-empty strings"
fi

build_candidate() {
  if [[ "$FLAGS_SUPPLIED" -eq 1 ]]; then
    jq --arg train "$TRAIN" --argjson flags "$FLAGS_JSON" '.releaseTrain = $train | .flagsIntroduced = $flags' "$STATE_FILE"
  else
    jq --arg train "$TRAIN" '.releaseTrain = $train' "$STATE_FILE"
  fi
}

if [[ "$MODE" == "dry-run" ]]; then
  build_candidate
  echo "release-train-metadata-assign: dry-run candidate for train '$TRAIN': $STATE_FILE" >&2
  exit 0
fi

CANDIDATE=""
LOCK_DIR="$STATE_DIR/.release-train-metadata.lock"
LOCK_HELD=0
cleanup() {
  [[ -z "$CANDIDATE" ]] || rm -f "$CANDIDATE"
  if [[ "$LOCK_HELD" -eq 1 ]]; then
    rmdir "$LOCK_DIR" 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

[[ ! -e "$LOCK_DIR" && ! -L "$LOCK_DIR" ]] || refuse "concurrent metadata writer lock is already held"
mkdir "$LOCK_DIR" 2>/dev/null || refuse "concurrent metadata writer lock could not be acquired"
LOCK_HELD=1
state_target_is_safe "$STATE_FILE" || refuse "state target drifted before mutation"
SOURCE_DIGEST="$(file_digest "$STATE_FILE")" || refuse "could not digest source state"
SOURCE_IDENTITY="$(file_identity "$STATE_FILE")" || refuse "could not identify source state"
FILE_MODE="$(stat -c '%a' "$STATE_FILE" 2>/dev/null || stat -f '%Lp' "$STATE_FILE" 2>/dev/null)" || refuse "could not read state.json file mode"

CANDIDATE="$(mktemp "$STATE_DIR/.release-train-metadata.XXXXXX")" || refuse "could not create same-directory candidate"
build_candidate >"$CANDIDATE" || refuse "could not build candidate"
jq -e 'type == "object"' "$CANDIDATE" >/dev/null 2>&1 || refuse "candidate is not a JSON object"

SOURCE_PROJECTION="$(jq -S 'del(.releaseTrain, .flagsIntroduced)' "$STATE_FILE")" || refuse "could not project source state"
CANDIDATE_PROJECTION="$(jq -S 'del(.releaseTrain, .flagsIntroduced)' "$CANDIDATE")" || refuse "could not project candidate state"
[[ "$SOURCE_PROJECTION" == "$CANDIDATE_PROJECTION" ]] || refuse "candidate changed non-owned state values"
[[ "$(jq -r '.releaseTrain // ""' "$CANDIDATE")" == "$TRAIN" ]] || refuse "candidate releaseTrain verification failed"
if [[ "$FLAGS_SUPPLIED" -eq 1 ]]; then
  [[ "$(jq -c '.flagsIntroduced' "$CANDIDATE")" == "$(jq -c '.' <<<"$FLAGS_JSON")" ]] || refuse "candidate flagsIntroduced verification failed"
fi

state_target_is_safe "$STATE_FILE" || refuse "state target drifted while candidate was built"
[[ "$(file_digest "$STATE_FILE")" == "$SOURCE_DIGEST" ]] || refuse "state changed concurrently; stale candidate will not be applied"
[[ "$(file_identity "$STATE_FILE")" == "$SOURCE_IDENTITY" ]] || refuse "state file identity changed concurrently; stale candidate will not be applied"

if jq -e --arg train "$TRAIN" --argjson supplied "$FLAGS_SUPPLIED" --argjson flags "${FLAGS_JSON:-[]}" '
  .releaseTrain == $train
  and ($supplied == 0 or .flagsIntroduced == $flags)
' "$STATE_FILE" >/dev/null 2>&1; then
  echo "release-train-metadata-assign: idempotent no-op for train '$TRAIN': $STATE_FILE"
  exit 0
fi

chmod "$FILE_MODE" "$CANDIDATE" || refuse "could not preserve state.json file mode"
state_target_is_safe "$STATE_FILE" || refuse "state target drifted before atomic replacement"
[[ "$(file_digest "$STATE_FILE")" == "$SOURCE_DIGEST" ]] || refuse "state changed concurrently before replacement; stale candidate will not be applied"
[[ "$(file_identity "$STATE_FILE")" == "$SOURCE_IDENTITY" ]] || refuse "state file identity changed before replacement; stale candidate will not be applied"
mv "$CANDIDATE" "$STATE_FILE" || refuse "atomic state.json replacement failed"
CANDIDATE=""
state_target_is_safe "$STATE_FILE" || refuse "state target is unsafe after atomic replacement"
echo "release-train-metadata-assign: applied metadata assignment for train '$TRAIN': $STATE_FILE"