#!/usr/bin/env bash
set -euo pipefail
umask 077

# Portable awk: the 3-arg match($0, /re/, arr) form used below is a GNU awk
# extension that BSD/macOS awk rejects. Prefer gawk when present (the framework's
# macOS toolchain provides it alongside gsed/ggrep/gtimeout).
if command -v gawk >/dev/null 2>&1; then awk() { command gawk "$@"; }; fi

# context-compactor.sh
# Compact a subagent RESULT-ENVELOPE into a single-line JSON ledger record
# suitable for inclusion in .specify/memory/bubbles.session.json under the
# `compactedHistory` array. Used by orchestrator agents (workflow, sprint,
# goal, iterate) to keep accumulated subagent output from blowing the
# context window.
#
# See: agents/bubbles_shared/operating-baseline.md
#      → "Context Compaction Discipline (Orchestrator Agents)"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPOSITORY_BINDING="$SCRIPT_DIR/repository-binding.sh"
GUARD_LIB="$SCRIPT_DIR/guard-lib.sh"
[[ -f "$GUARD_LIB" ]] || { echo "context-compactor: guard-lib.sh missing at $GUARD_LIB" >&2; exit 2; }
# shellcheck source=/dev/null
source "$GUARD_LIB"

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/context-compactor.sh \
  [--session-id <id> --session-control-file <path> --binding-packet-file <path>] \
  <raw-result-file>

Reads a raw subagent RESULT-ENVELOPE (markdown is preferred; minimal JSON
also accepted) and emits a single-line compact JSON record on stdout.

Arguments:
  raw-result-file   Path to the file containing the raw RESULT-ENVELOPE.

Options:
  --session-id <id>              Current interactive session id.
  --session-control-file <path>  Host-private authoritative control record.
  --binding-packet-file <path>   Current local actionable binding packet.
                                 Supply all three options or none.
  -h, --help        Print this usage and exit.

Behavior:
  - Extracts: agent, outcome, featureDir, scopeIds, dodItems,
    artifactsCreated, artifactsUpdated, evidenceRefs, nextRequiredOwner,
    blockedReason, timestamp, rawPointer.
  - With repository binding, preserves repositoryRoot, repositoryAlias, and
    the exact nested repositoryResolution packet for validation on resume.
    Legacy flattened binding fields remain present for existing readers.
  - Without repository binding, emits stdout only and performs no
    repository-local session mutation.
  - Long evidence values are truncated to the first 5 lines with a
    "...N more lines" sentinel; the rawPointer field preserves the path
    back to the original raw envelope so an operator can drill in.
  - Idempotent: running twice on the same input file yields byte-identical
    output. The timestamp is derived from the file mtime, never the wall
    clock.
  - Missing optional fields are recorded as JSON null (no crash).

Reference:
  agents/bubbles_shared/operating-baseline.md
    -> "Context Compaction Discipline (Orchestrator Agents)"
EOF
}

SESSION_ID=""
SESSION_CONTROL_FILE=""
BINDING_PACKET_FILE=""
VALIDATED_PACKET_FILE=""
VALIDATED_PACKET=""
COMPACTOR_SESSION_FILE=""
SCENARIO_FILE=""
NODE_ID=""
raw_file=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --session-id)
      [[ $# -ge 2 ]] || { echo "context-compactor: --session-id requires a value" >&2; exit 2; }
      SESSION_ID="$2"
      shift 2
      ;;
    --session-control-file)
      [[ $# -ge 2 ]] || { echo "context-compactor: --session-control-file requires a value" >&2; exit 2; }
      SESSION_CONTROL_FILE="$2"
      shift 2
      ;;
    --binding-packet-file)
      [[ $# -ge 2 ]] || { echo "context-compactor: --binding-packet-file requires a value" >&2; exit 2; }
      BINDING_PACKET_FILE="$2"
      shift 2
      ;;
    --scenario-file)
      [[ $# -ge 2 ]] || { echo "context-compactor: --scenario-file requires a value" >&2; exit 2; }
      SCENARIO_FILE="$2"
      shift 2
      ;;
    --node-id)
      [[ $# -ge 2 ]] || { echo "context-compactor: --node-id requires a value" >&2; exit 2; }
      NODE_ID="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --*)
      echo "context-compactor: unknown option: $1" >&2
      exit 2
      ;;
    *)
      [[ -z "$raw_file" ]] || { echo "context-compactor: exactly one raw-result-file is required" >&2; exit 2; }
      raw_file="$1"
      shift
      ;;
  esac
done

[[ -n "$raw_file" ]] || { usage >&2; exit 2; }

BINDING_REQUIRED=false
if [[ -n "$SESSION_ID" || -n "$SESSION_CONTROL_FILE" || -n "$BINDING_PACKET_FILE" ]]; then
  BINDING_REQUIRED=true
  [[ -n "$SESSION_ID" ]] || { echo "context-compactor: --session-id is required with repository binding" >&2; exit 2; }
  [[ -n "$SESSION_CONTROL_FILE" ]] || { echo "context-compactor: --session-control-file is required with repository binding" >&2; exit 2; }
  [[ -n "$BINDING_PACKET_FILE" ]] || { echo "context-compactor: --binding-packet-file is required with repository binding" >&2; exit 2; }
  if [[ -n "$SCENARIO_FILE" && -z "$NODE_ID" ]] || [[ -z "$SCENARIO_FILE" && -n "$NODE_ID" ]]; then
    echo "context-compactor: goal-node validation requires both --scenario-file and --node-id" >&2
    exit 2
  fi
  [[ -f "$REPOSITORY_BINDING" ]] || { echo "context-compactor: repository binding validator missing at $REPOSITORY_BINDING" >&2; exit 2; }
  VALIDATED_PACKET_FILE="$(mktemp)" || { echo "context-compactor: unable to create immutable packet snapshot" >&2; exit 2; }
  trap 'rm -f "$VALIDATED_PACKET_FILE"' EXIT INT TERM
  cp -- "$BINDING_PACKET_FILE" "$VALIDATED_PACKET_FILE" || {
    echo "context-compactor: unable to capture binding packet" >&2
    exit 2
  }
  chmod 600 "$VALIDATED_PACKET_FILE"
  set +e
  if [[ -n "$SCENARIO_FILE" ]]; then
    BINDING_OUTPUT="$(bash "$REPOSITORY_BINDING" validate-packet \
      --session-id "$SESSION_ID" \
      --session-control-file "$SESSION_CONTROL_FILE" \
      --packet-file "$VALIDATED_PACKET_FILE" \
      --scenario-file "$SCENARIO_FILE" --node-id "$NODE_ID" 2>&1)"
  else
    BINDING_OUTPUT="$(bash "$REPOSITORY_BINDING" validate-packet \
      --session-id "$SESSION_ID" \
      --session-control-file "$SESSION_CONTROL_FILE" \
      --packet-file "$VALIDATED_PACKET_FILE" 2>&1)"
  fi
  BINDING_RC=$?
  set -e
  if [[ "$BINDING_RC" -ne 0 ]]; then
    printf '%s\n' "$BINDING_OUTPUT" >&2
    exit "$BINDING_RC"
  fi
  VALIDATED_PACKET="$(cat -- "$VALIDATED_PACKET_FILE")" || exit 2
  BINDING_REPOSITORY_ROOT="$(jq -r '.repositoryRoot' <<< "$VALIDATED_PACKET")"
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
    # An empty component comes from a redundant separator (a trailing-slash
    # XDG_RUNTIME_DIR yields "//"); it is not a path element to inspect.
    [[ -n "$component" ]] || continue
    [[ "$component" != "." && "$component" != ".." ]] || return 0
    cursor="$cursor/$component"
    [[ ! -L "$cursor" ]] || return 0
    [[ -e "$cursor" ]] || return 1
  done
  return 1
}

if [[ "$BINDING_REQUIRED" == true ]]; then
  COMPACTOR_SESSION_FILE="$BINDING_REPOSITORY_ROOT/.specify/memory/bubbles.session.json"
  if path_has_symlink_component "$COMPACTOR_SESSION_FILE"; then
    echo "context-compactor: repository session path must not traverse symlink components" >&2
    exit 1
  fi
fi

if [[ ! -f "$raw_file" ]]; then
  echo "context-compactor: input file not found: $raw_file" >&2
  exit 1
fi

if [[ "$BINDING_REQUIRED" == false ]] &&
   grep -Eq '^[[:space:]]*[" ]*(\*\*)?(repositoryRoot|repositoryAlias|repositoryResolution)(\*\*)?[" ]*[[:space:]]*:' "$raw_file"; then
  echo "context-compactor: repository binding inputs are required for a repository-sensitive result" >&2
  echo "  Supply --session-id, --session-control-file, and --binding-packet-file." >&2
  exit 2
fi

# Resolve to absolute path for deterministic rawPointer. Preserve an already-
# absolute path verbatim — do NOT canonicalize symlinks: on macOS `readlink -f`
# rewrites /var/... -> /private/var/..., diverging from the path the caller (and
# its provenance records) actually hold.
if [[ "$raw_file" = /* ]]; then
  raw_abs="$raw_file"
else
  raw_abs="$PWD/$raw_file"
fi

# JSON-escape stdin: handles backslash, double-quote, tab, CR; joins
# multi-line input with literal \n.
json_escape() {
  awk '
    BEGIN { result = "" }
    {
      line = $0
      gsub(/\\/, "\\\\", line)
      gsub(/"/, "\\\"", line)
      gsub(/\t/, "\\t", line)
      gsub(/\r/, "\\r", line)
      if (NR > 1) {
        result = result "\\n"
      }
      result = result line
    }
    END { printf "%s", result }
  '
}

# Extract a named field from the input file.
# Accepts the following on-disk shapes:
#   field: value
#   **field:** value
#   "field": value
#   field:
#     - value-line-1
#     - value-line-2
extract_field() {
  local field="$1"
  local file="$2"
  awk -v field="$field" '
    function trim(s) {
      sub(/^[[:space:]]+/, "", s)
      sub(/[[:space:]]+$/, "", s)
      return s
    }
    BEGIN {
      in_block = 0
      result = ""
      pat = "^[[:space:]]*[\"]?(\\*\\*)?" field "(\\*\\*)?[\"]?[[:space:]]*:[[:space:]]*"
    }
    {
      line = $0
      if (in_block == 0 && match(line, pat)) {
        value = substr(line, RSTART + RLENGTH)
        value = trim(value)
        sub(/,$/, "", value)
        sub(/^"/, "", value); sub(/"$/, "", value)
        if (value != "" && value != "null") {
          print value
          exit
        }
        in_block = 1
        next
      }
      if (in_block == 1) {
        if (match(line, /^[[:space:]]*-[[:space:]]*(.*)$/, m)) {
          item = trim(m[1])
          if (item != "") {
            if (result != "") result = result "\n"
            result = result item
          }
          next
        }
        if (line ~ /^[[:space:]]*$/) { next }
        # Any non-bullet, non-blank line ends the block.
        if (result != "") {
          print result
          result = ""
        }
        in_block = 0
        exit
      }
    }
    END {
      if (in_block == 1 && result != "") print result
    }
  ' "$file"
}

# Try multiple variant names; return the first non-empty match.
extract_any() {
  local file="$1"
  shift
  local value=""
  for name in "$@"; do
    value="$(extract_field "$name" "$file")"
    if [[ -n "$value" ]]; then
      printf '%s' "$value"
      return
    fi
  done
}

# Binding fields are mandatory scalar provenance. Unlike optional envelope
# fields, an explicit JSON null must remain distinguishable from omission.
extract_binding_scalar() {
  local field="$1"
  local file="$2"
  awk -v field="$field" '
    function trim(s) {
      sub(/^[[:space:]]+/, "", s)
      sub(/[[:space:]]+$/, "", s)
      return s
    }
    BEGIN {
      pat = "^[[:space:]]*[\"]?(\\*\\*)?" field "(\\*\\*)?[\"]?[[:space:]]*:[[:space:]]*"
    }
    match($0, pat) {
      value = trim(substr($0, RSTART + RLENGTH))
      sub(/,$/, "", value)
      sub(/^"/, "", value)
      sub(/"$/, "", value)
      print value
      exit
    }
  ' "$file"
}

# Truncate multi-line text to first N lines + "...K more lines" sentinel.
truncate_text() {
  local text="$1"
  local max_lines="${2:-5}"
  local total_lines
  total_lines="$(printf '%s' "$text" | awk 'END { print NR + 0 }')"
  if (( total_lines <= max_lines )); then
    printf '%s' "$text"
    return
  fi
  local head_lines
  head_lines="$(printf '%s' "$text" | awk -v n="$max_lines" 'NR <= n')"
  local remaining=$((total_lines - max_lines))
  printf '%s\n...%s more lines' "$head_lines" "$remaining"
}

# Deterministic timestamp from file mtime (UTC ISO-8601). Required for
# idempotency: never reads the wall clock.
file_timestamp() {
  local f="$1"
  local epoch
  epoch="$(bubbles_file_mtime_epoch "$f")" || {
    echo "context-compactor: unable to read mtime of $f" >&2
    exit 1
  }
  if date -u -d "@$epoch" '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null; then
    return 0
  fi
  date -u -r "$epoch" '+%Y-%m-%dT%H:%M:%SZ'
}

agent_v="$(extract_any "$raw_file" agent Agent)"
outcome_v="$(extract_any "$raw_file" outcome Outcome)"
feature_dir_v="$(extract_any "$raw_file" featureDir feature_dir)"
scope_ids_v="$(extract_any "$raw_file" scopeIds scope_ids scopes)"
dod_items_v="$(extract_any "$raw_file" dodItems dod_items dod)"
artifacts_created_v="$(extract_any "$raw_file" artifactsCreated artifacts_created files_created)"
artifacts_updated_v="$(extract_any "$raw_file" artifactsUpdated artifacts_updated files_modified)"
evidence_raw="$(extract_any "$raw_file" evidenceRefs evidence_refs evidence)"
next_owner_v="$(extract_any "$raw_file" nextRequiredOwner next_required_owner nextOwner)"
blocked_reason_v="$(extract_any "$raw_file" blockedReason blocked_reason blocker)"
timestamp_v="$(file_timestamp "$raw_file")"
evidence_compact="$(truncate_text "$evidence_raw" 5)"

if [[ "$BINDING_REQUIRED" == true ]]; then
  repository_root_v="$(extract_binding_scalar repositoryRoot "$raw_file")"
  repository_alias_v="$(extract_binding_scalar repositoryAlias "$raw_file")"
  session_id_v="$(extract_binding_scalar sessionId "$raw_file")"
  decision_id_v="$(extract_binding_scalar decisionId "$raw_file")"
  control_revision_v="$(extract_binding_scalar controlRevision "$raw_file")"
  control_path_digest_v="$(extract_binding_scalar controlPathDigest "$raw_file")"
  authority_v="$(extract_binding_scalar authority "$raw_file")"
  transition_v="$(extract_binding_scalar transition "$raw_file")"
  scope_kind_v="$(extract_binding_scalar scopeKind "$raw_file")"
  scope_id_v="$(extract_binding_scalar scopeId "$raw_file")"
  target_kind_v="$(extract_binding_scalar targetKind "$raw_file")"
  path_visibility_v="$(extract_binding_scalar pathVisibility "$raw_file")"
  actionable_v="$(extract_binding_scalar actionable "$raw_file")"

  if [[ -z "$repository_root_v" || -z "$repository_alias_v" || -z "$session_id_v" ||
        -z "$decision_id_v" || -z "$control_revision_v" || -z "$control_path_digest_v" || -z "$authority_v" ||
        -z "$transition_v" || -z "$scope_kind_v" || -z "$scope_id_v" ||
      -z "$target_kind_v" || -z "$path_visibility_v" || -z "$actionable_v" ]] ||
     ! jq -e \
       --arg root "$repository_root_v" \
       --arg alias "$repository_alias_v" \
       --arg session "$session_id_v" \
       --arg decision "$decision_id_v" \
       --arg revision "$control_revision_v" \
      --arg control_path_digest "$control_path_digest_v" \
       --arg authority "$authority_v" \
       --arg transition "$transition_v" \
       --arg scope_kind "$scope_kind_v" \
       --arg scope_id "$scope_id_v" \
        --arg target_kind "$target_kind_v" \
       --arg visibility "$path_visibility_v" \
       --arg actionable "$actionable_v" \
       '.repositoryRoot == $root and
        .repositoryAlias == $alias and
        .repositoryResolution.sessionId == $session and
        .repositoryResolution.decisionId == $decision and
        (.repositoryResolution.controlRevision | tostring) == $revision and
        .repositoryResolution.controlPathDigest == $control_path_digest and
        .repositoryResolution.authority == $authority and
        .repositoryResolution.transition == $transition and
        .repositoryResolution.scopeKind == $scope_kind and
        ((.repositoryResolution.scopeId == null and $scope_id == "null") or
         (.repositoryResolution.scopeId | tostring) == $scope_id) and
        .repositoryResolution.targetKind == $target_kind and
        .repositoryResolution.pathVisibility == $visibility and
        (.repositoryResolution.actionable | tostring) == $actionable' \
      <<< "$VALIDATED_PACKET" >/dev/null 2>&1; then
    printf 'REPOSITORY PACKET REFUSED reason=BOUNDARY_CONFLICT actionable=false\n' >&2
    exit 1
  fi
fi

emit() {
  local value="$1"
  if [[ -z "$value" ]]; then
    printf 'null'
  else
    printf '"%s"' "$(printf '%s' "$value" | json_escape)"
  fi
}

emit_nullable_binding() {
  local value="$1"
  if [[ "$value" == "null" ]]; then
    printf 'null'
  else
    emit "$value"
  fi
}

{
  printf '{'
  printf '"agent":%s,' "$(emit "$agent_v")"
  printf '"outcome":%s,' "$(emit "$outcome_v")"
  printf '"featureDir":%s,' "$(emit "$feature_dir_v")"
  printf '"scopeIds":%s,' "$(emit "$scope_ids_v")"
  printf '"dodItems":%s,' "$(emit "$dod_items_v")"
  printf '"artifactsCreated":%s,' "$(emit "$artifacts_created_v")"
  printf '"artifactsUpdated":%s,' "$(emit "$artifacts_updated_v")"
  printf '"evidenceRefs":%s,' "$(emit "$evidence_compact")"
  printf '"nextRequiredOwner":%s,' "$(emit "$next_owner_v")"
  printf '"blockedReason":%s,' "$(emit "$blocked_reason_v")"
  if [[ "$BINDING_REQUIRED" == true ]]; then
    printf '"repositoryRoot":%s,' "$(emit "$repository_root_v")"
    printf '"repositoryAlias":%s,' "$(emit "$repository_alias_v")"
    printf '"repositoryResolution":{'
    printf '"sessionId":%s,' "$(emit "$session_id_v")"
    printf '"decisionId":%s,' "$(emit "$decision_id_v")"
    printf '"controlRevision":%s,' "$control_revision_v"
    printf '"controlPathDigest":%s,' "$(emit "$control_path_digest_v")"
    printf '"authority":%s,' "$(emit "$authority_v")"
    printf '"transition":%s,' "$(emit "$transition_v")"
    printf '"scopeKind":%s,' "$(emit "$scope_kind_v")"
    printf '"scopeId":%s,' "$(emit_nullable_binding "$scope_id_v")"
    printf '"targetKind":%s,' "$(emit "$target_kind_v")"
    printf '"pathVisibility":%s,' "$(emit "$path_visibility_v")"
    printf '"actionable":%s' "$actionable_v"
    printf '},'
    # Retain the original flattened fields for additive compatibility with
    # readers introduced before resumable repository packets.
    printf '"sessionId":%s,' "$(emit "$session_id_v")"
    printf '"decisionId":%s,' "$(emit "$decision_id_v")"
    printf '"controlRevision":%s,' "$control_revision_v"
    printf '"controlPathDigest":%s,' "$(emit "$control_path_digest_v")"
    printf '"authority":%s,' "$(emit "$authority_v")"
    printf '"transition":%s,' "$(emit "$transition_v")"
    printf '"scopeKind":%s,' "$(emit "$scope_kind_v")"
    printf '"scopeId":%s,' "$(emit_nullable_binding "$scope_id_v")"
    printf '"targetKind":%s,' "$(emit "$target_kind_v")"
    printf '"pathVisibility":%s,' "$(emit "$path_visibility_v")"
    printf '"actionable":%s,' "$actionable_v"
  fi
  printf '"timestamp":"%s",' "$timestamp_v"
  printf '"rawPointer":"%s"' "$(printf '%s' "$raw_abs" | json_escape)"
  printf '}\n'
}

# --- Gate G083: additively stamp `compactedAt` on the matching ------------
# `envelopesReceived[]` entry, if any.
#
# This is a BEST-EFFORT, ADDITIVE side effect. The primary stdout contract
# (single-line compact JSON record) is preserved unconditionally. This
# block:
#   - Is a clean no-op if `jq` is missing.
#   - Is a clean no-op if `.specify/memory/bubbles.session.json` is missing.
#   - Is a clean no-op if no `envelopesReceived[]` entry exists whose
#     `rawPointer` matches `$raw_abs`.
#   - Stamps `compactedAt` only on entries that currently lack one
#     (idempotent: re-running on an already-compacted entry is a no-op).
#
# This field is consumed by `bubbles/scripts/compaction-discipline-guard.sh`
# (Gate G083), which treats any over-budget envelope WITHOUT `compactedAt`
# as a violation.
#
# Errors from this block MUST NOT fail the script — the operator could be
# running the compactor against a one-off raw envelope file outside any
# repo (e.g., to inspect a saved transition packet). We swallow any
# failure on stderr and exit 0.
if command -v jq >/dev/null 2>&1 && [[ "$BINDING_REQUIRED" == true ]]; then
  # A repository-local mutation derives its target only from the validated
  # actionable packet. Unbound use remains a pure stdout transformation.
  _comp_repo_root="$BINDING_REPOSITORY_ROOT"
  if [[ -n "$_comp_repo_root" ]]; then
    _comp_session_file="$COMPACTOR_SESSION_FILE"
    if [[ -f "$_comp_session_file" ]]; then
      _comp_now="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
      _comp_tmp="$(mktemp "$(dirname "$_comp_session_file")/.bubbles-session.XXXXXX" 2>/dev/null || true)"
      if [[ -n "$_comp_tmp" ]]; then
        if jq \
            --arg rawPointer "$raw_abs" \
            --arg compactedAt "$_comp_now" \
            '
            . as $root
            | if ($root.envelopesReceived // []) | type == "array" then
                $root + {
                  envelopesReceived: (
                    ($root.envelopesReceived // [])
                    | map(
                        if (.rawPointer // "") == $rawPointer
                           and (.compactedAt == null or (.compactedAt // "") == "")
                        then . + { compactedAt: $compactedAt }
                        else .
                        end
                      )
                  )
                }
              else
                $root
              end
            ' "$_comp_session_file" > "$_comp_tmp" 2>/dev/null; then
          mv "$_comp_tmp" "$_comp_session_file" 2>/dev/null || rm -f "$_comp_tmp" 2>/dev/null || true
        else
          rm -f "$_comp_tmp" 2>/dev/null || true
        fi
      fi
    fi
  fi
fi
