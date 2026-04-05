#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
WORKFLOWS_FILE="$REPO_ROOT/bubbles/workflows.yaml"
LESSONS_FILE="$REPO_ROOT/.specify/memory/lessons.md"
PROPOSALS_FILE="$REPO_ROOT/.specify/memory/skill-proposals.md"
DISMISSED_FILE="$REPO_ROOT/.specify/memory/skill-proposals-dismissed.md"

threshold_from_registry() {
  local threshold
  threshold="$({ grep -m1 '^  triggerThreshold:' "$WORKFLOWS_FILE" | sed -E 's/.*: *([0-9]+).*/\1/'; } || true)"
  if [[ -n "$threshold" ]]; then
    printf '%s\n' "$threshold"
  else
    printf '3\n'
  fi
}

normalize_lessons() {
  local threshold="$1"

  if [[ ! -f "$LESSONS_FILE" ]]; then
    return 0
  fi

  awk -v threshold="$threshold" '
    BEGIN { in_code = 0 }
    /^```/ { in_code = !in_code; next }
    in_code { next }
    /^[[:space:]]*#/ { next }
    /^[[:space:]]*$/ { next }
    {
      line = $0
      sub(/^[[:space:]]*[-*+]/, "", line)
      sub(/^[[:space:]]*[0-9]+[.)][[:space:]]*/, "", line)
      gsub(/[[:space:]]+/, " ", line)
      sub(/^[[:space:]]+/, "", line)
      sub(/[[:space:]]+$/, "", line)
      line = tolower(line)
      if (length(line) >= 20) {
        counts[line]++
      }
    }
    END {
      for (line in counts) {
        if (counts[line] >= threshold) {
          printf "%d|%s\n", counts[line], line
        }
      }
    }
  ' "$LESSONS_FILE" | sort -t'|' -k1,1nr -k2,2
}

slugify() {
  printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//; s/-{2,}/-/g'
}

write_proposals() {
  local threshold="$1"
  local temp_file
  local proposals

  mkdir -p "$(dirname "$PROPOSALS_FILE")"
  proposals="$(normalize_lessons "$threshold")"

  if [[ -z "$proposals" ]]; then
    rm -f "$PROPOSALS_FILE"
    return 0
  fi

  temp_file="$(mktemp)"
  {
    echo "# Skill Proposals"
    echo
    echo "Generated: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    echo "Trigger threshold: ${threshold} repeated lesson entries"
    echo
    while IFS='|' read -r count pattern; do
      [[ -n "$pattern" ]] || continue
      local_slug="$(slugify "$pattern")"
      if [[ -z "$local_slug" ]]; then
        local_slug="generated-skill"
      fi
      echo "## Skill Proposal: ${local_slug}"
      echo "- Pattern: ${pattern}"
      echo "- Observed: ${count} times"
      echo "- Proposed skill: .github/skills/${local_slug}/SKILL.md"
      echo "- Action: Create / Dismiss / Later"
      echo
    done <<< "$proposals"
  } > "$temp_file"

  mv "$temp_file" "$PROPOSALS_FILE"
}

show_proposals() {
  local threshold
  threshold="$(threshold_from_registry)"
  write_proposals "$threshold"

  if [[ -f "$PROPOSALS_FILE" ]]; then
    cat "$PROPOSALS_FILE"
  else
    echo "No skill proposals. Repeated lesson patterns have not crossed the threshold yet."
  fi
}

dismiss_proposals() {
  if [[ ! -f "$PROPOSALS_FILE" ]]; then
    echo "No skill proposals to dismiss."
    return 0
  fi

  mkdir -p "$(dirname "$DISMISSED_FILE")"
  {
    echo "## Dismissed $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    cat "$PROPOSALS_FILE"
    echo
  } >> "$DISMISSED_FILE"

  rm -f "$PROPOSALS_FILE"
  echo "Dismissed all pending skill proposals."
}

case "${1:-show}" in
  show|refresh)
    show_proposals
    ;;
  dismiss|--dismiss)
    dismiss_proposals
    ;;
  *)
    echo "Usage: $(basename "$0") [show|refresh|dismiss]" >&2
    exit 1
    ;;
esac