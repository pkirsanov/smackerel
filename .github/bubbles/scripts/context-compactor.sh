#!/usr/bin/env bash
set -euo pipefail

# context-compactor.sh
# Compact a subagent RESULT-ENVELOPE into a single-line JSON ledger record
# suitable for inclusion in .specify/memory/bubbles.session.json under the
# `compactedHistory` array. Used by orchestrator agents (workflow, sprint,
# goal, iterate) to keep accumulated subagent output from blowing the
# context window.
#
# See: agents/bubbles_shared/operating-baseline.md
#      → "Context Compaction Discipline (Orchestrator Agents)"

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/context-compactor.sh <raw-result-file>

Reads a raw subagent RESULT-ENVELOPE (markdown is preferred; minimal JSON
also accepted) and emits a single-line compact JSON record on stdout.

Arguments:
  raw-result-file   Path to the file containing the raw RESULT-ENVELOPE.

Options:
  -h, --help        Print this usage and exit.

Behavior:
  - Extracts: agent, outcome, featureDir, scopeIds, dodItems,
    artifactsCreated, artifactsUpdated, evidenceRefs, nextRequiredOwner,
    blockedReason, timestamp, rawPointer.
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

if [[ $# -eq 0 ]]; then
  usage >&2
  exit 2
fi

case "$1" in
  -h|--help)
    usage
    exit 0
    ;;
esac

raw_file="$1"
if [[ ! -f "$raw_file" ]]; then
  echo "context-compactor: input file not found: $raw_file" >&2
  exit 1
fi

# Resolve to absolute path for deterministic rawPointer.
if command -v readlink >/dev/null 2>&1; then
  raw_abs="$(readlink -f "$raw_file" 2>/dev/null || printf '%s' "$raw_file")"
else
  raw_abs="$raw_file"
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
  if epoch="$(stat -c %Y "$f" 2>/dev/null)"; then
    date -u -d "@$epoch" '+%Y-%m-%dT%H:%M:%SZ'
  elif epoch="$(stat -f %m "$f" 2>/dev/null)"; then
    date -u -r "$epoch" '+%Y-%m-%dT%H:%M:%SZ'
  else
    echo "context-compactor: unable to read mtime of $f" >&2
    exit 1
  fi
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

emit() {
  local value="$1"
  if [[ -z "$value" ]]; then
    printf 'null'
  else
    printf '"%s"' "$(printf '%s' "$value" | json_escape)"
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
  printf '"timestamp":"%s",' "$timestamp_v"
  printf '"rawPointer":"%s"' "$(printf '%s' "$raw_abs" | json_escape)"
  printf '}\n'
}
