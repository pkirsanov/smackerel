#!/usr/bin/env bash
# Effective Prompt-Bundle Measurement (IMP-100 Phase 6 / IMP-020 S5 — AF-006)
# ---------------------------------------------------------------------------
# An agent's effective loaded prompt is NOT just its agent.md — it is that file
# PLUS every shared contract it transitively references (agents/bubbles_shared/*.md).
# This tool measures that closure so the real loaded bundle is observable (and can
# be tracked / budgeted) instead of guessed from the agent file alone.
#
# It resolves the transitive closure of `bubbles_shared/<name>.md` references
# starting from the agent file (bounded, cycle-safe), and reports each file's
# bytes/lines plus the totals. It also counts skill pointers (`skills/...`
# references) as a secondary surface signal. Pure measurement — no pass/fail.
#
# Usage:
#   bash bubbles/scripts/effective-bundle-measure.sh <agent-file> [--agents-dir <dir>]
#
# Output: JSON on stdout { agent, files:[{path,bytes,lines}], totalFiles,
#         totalBytes, totalLines, skillPointers }.
#
# Exit codes:
#   0  measurement produced
#   2  usage / runtime error
set -euo pipefail

AGENT_FILE=""
AGENTS_DIR=""

usage() {
  cat <<'EOF'
Usage: effective-bundle-measure.sh <agent-file> [--agents-dir <dir>]

Measures an agent's effective loaded prompt bundle: the agent file plus every
transitively-referenced agents/bubbles_shared/*.md contract. Emits a JSON summary.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h | --help)
      usage
      exit 0
      ;;
    --agents-dir)
      [[ $# -ge 2 ]] || { echo "effective-bundle-measure: --agents-dir requires a value" >&2; exit 2; }
      AGENTS_DIR="$2"
      shift 2
      ;;
    --*)
      echo "effective-bundle-measure: unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -n "$AGENT_FILE" ]]; then
        echo "effective-bundle-measure: only one agent file may be supplied" >&2
        exit 2
      fi
      AGENT_FILE="$1"
      shift
      ;;
  esac
done

if [[ -z "$AGENT_FILE" ]]; then
  echo "effective-bundle-measure: missing required <agent-file>" >&2
  usage >&2
  exit 2
fi
if [[ ! -f "$AGENT_FILE" ]]; then
  echo "effective-bundle-measure: agent file not found: $AGENT_FILE" >&2
  exit 2
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "effective-bundle-measure: jq is required but not found in PATH" >&2
  exit 2
fi

# The agents dir is where bubbles_shared/ lives (default: the agent file's dir).
[[ -n "$AGENTS_DIR" ]] || AGENTS_DIR="$(cd "$(dirname "$AGENT_FILE")" && pwd)"

seen_file="$(mktemp)"
trap 'rm -f "$seen_file"' EXIT INT TERM

# Worklist BFS over the transitive bubbles_shared reference closure.
queue=("$AGENT_FILE")
files_json="[]"
skill_pointers=0

while [[ "${#queue[@]}" -gt 0 ]]; do
  current="${queue[0]}"
  queue=("${queue[@]:1}")
  [[ -f "$current" ]] || continue
  abs="$(cd "$(dirname "$current")" && pwd)/$(basename "$current")"
  if grep -qxF "$abs" "$seen_file" 2>/dev/null; then
    continue
  fi
  printf '%s\n' "$abs" >> "$seen_file"

  bytes="$(wc -c < "$current" | tr -d ' ')"
  lines="$(wc -l < "$current" | tr -d ' ')"
  files_json="$(printf '%s' "$files_json" | jq --arg p "$abs" --argjson b "$bytes" --argjson l "$lines" '. + [{path: $p, bytes: $b, lines: $l}]')"

  # Count skill pointers on this file (secondary surface signal).
  sp="$( { grep -oE 'skills/[A-Za-z0-9._/-]+' "$current" 2>/dev/null || true; } | sort -u | wc -l | tr -d ' ')"
  skill_pointers=$((skill_pointers + sp))

  # Enqueue transitively-referenced shared contracts.
  while IFS= read -r ref; do
    [[ -n "$ref" ]] || continue
    name="${ref#bubbles_shared/}"
    candidate="$AGENTS_DIR/bubbles_shared/$name"
    if [[ -f "$candidate" ]]; then
      queue+=("$candidate")
    fi
  done < <({ grep -oE 'bubbles_shared/[A-Za-z0-9._-]+\.md' "$current" 2>/dev/null || true; } | sort -u)
done

jq -n \
  --arg agent "$AGENT_FILE" \
  --argjson files "$files_json" \
  --argjson skillPointers "$skill_pointers" \
  '{
    agent: $agent,
    files: $files,
    totalFiles: ($files | length),
    totalBytes: ([$files[].bytes] | add // 0),
    totalLines: ([$files[].lines] | add // 0),
    skillPointers: $skillPointers
  }'
exit 0
