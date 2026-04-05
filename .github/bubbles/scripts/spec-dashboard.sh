#!/usr/bin/env bash
# Bubbles Spec Progress Dashboard
# Generic, project-agnostic spec status overview across all specs/ folders.
# Usage: bash bubbles/scripts/spec-dashboard.sh [specs-dir]
#
# Output: Summary table of all specs with status, workflow mode, scope counts,
#         and completion percentage.

set -uo pipefail

# Source fun mode support
source "$(dirname "${BASH_SOURCE[0]}")/fun-mode.sh"

SPECS_DIR="${1:-specs}"

if [[ ! -d "$SPECS_DIR" ]]; then
  echo "Error: specs directory '$SPECS_DIR' not found"
  exit 1
fi

# Colors (disable if not a terminal)
if [[ -t 1 ]]; then
  GREEN='\033[0;32m'
  YELLOW='\033[0;33m'
  RED='\033[0;31m'
  BLUE='\033[0;34m'
  NC='\033[0m'
else
  GREEN='' YELLOW='' RED='' BLUE='' NC=''
fi

total_specs=0
done_specs=0
in_progress_specs=0
blocked_specs=0
not_started_specs=0
other_specs=0

printf "\n${BLUE}═══════════════════════════════════════════════════════════════════════════${NC}\n"
printf "${BLUE}  Bubbles Spec Progress Dashboard${NC}\n"
printf "${BLUE}═══════════════════════════════════════════════════════════════════════════${NC}\n\n"

printf "%-40s %-16s %-24s %-8s %-8s\n" "SPEC" "STATUS" "MODE" "SCOPES" "DONE"
printf "%-40s %-16s %-24s %-8s %-8s\n" "────" "──────" "────" "──────" "────"

for state_file in $(find "$SPECS_DIR" -maxdepth 2 -name "state.json" -not -path "*/bugs/*" | sort); do
  spec_dir="$(dirname "$state_file")"
  spec_name="$(basename "$spec_dir")"

  # Extract status (prefer certification.status when present)
  status="$({
    grep -A12 '"certification"' "$state_file" 2>/dev/null \
      | grep -m1 '"status"' \
      | sed -E 's/.*"status"[[:space:]]*:[[:space:]]*"([^"]+)"/\1/'
  } || true)"
  if [[ -z "$status" ]]; then
    status="$(grep -oE '"status"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" 2>/dev/null | head -1 | sed -E 's/.*"([^"]+)"$/\1/' || echo "unknown")"
  fi

  # Extract workflow mode
  mode="$(grep -oE '"workflowMode"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" 2>/dev/null | head -1 | sed -E 's/.*"([^"]+)"$/\1/' || true)"
  if [[ -z "$mode" ]]; then
    mode="$(grep -oE '"mode"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" 2>/dev/null | head -1 | sed -E 's/.*"([^"]+)"$/\1/' || echo "-")"
  fi

  # Count completed scopes (prefer certification.completedScopes when present)
  completed_scopes_block="$({
    grep -A40 '"certification"' "$state_file" 2>/dev/null \
      | awk '/"completedScopes"/{capture=1} capture{print} capture && /\]/{exit}'
  } || true)"
  if [[ -z "$completed_scopes_block" ]]; then
    completed_scopes_block="$({
      awk '/"completedScopes"/{capture=1} capture{print} capture && /\]/{exit}' "$state_file"
    } || true)"
  fi
  if [[ -n "$completed_scopes_block" ]]; then
    done_count="$(echo "$completed_scopes_block" | grep -cE '"[^"]+"' || echo "0")"
  else
    done_count=0
  fi

  # Count total scopes from scopes.md
  total_scopes=0
  if [[ -f "$spec_dir/scopes.md" ]]; then
    total_scopes="$(grep -cE '^## Scope [0-9]+|^# Scope [0-9]+|^### Scope [0-9]+|Status:' "$spec_dir/scopes.md" 2>/dev/null || echo "0")"
    # Rough: count Status: lines as scope count
    status_lines="$(grep -c 'Status:' "$spec_dir/scopes.md" 2>/dev/null || echo "0")"
    if [[ "$status_lines" -gt 0 ]]; then
      total_scopes="$status_lines"
    fi
  fi
  if [[ -d "$spec_dir/scopes" ]]; then
    total_scopes="$(find "$spec_dir/scopes" -name "scope.md" | wc -l)"
  fi

  scope_info="${done_count}/${total_scopes}"

  # Color status
  case "$status" in
    done) status_display="${GREEN}done${NC}"; ((done_specs++)) ;;
    in_progress) status_display="${YELLOW}in_progress${NC}"; ((in_progress_specs++)) ;;
    blocked) status_display="${RED}blocked${NC}"; ((blocked_specs++)) ;;
    not_started) status_display="not_started"; ((not_started_specs++)) ;;
    *) status_display="$status"; ((other_specs++)) ;;
  esac

  ((total_specs++))

  printf "%-40s %-16b %-24s %-8s %-8s\n" "$spec_name" "$status_display" "${mode:--}" "$scope_info" ""
done

# Bug summary
bug_total=0
bug_done=0
bug_open=0
for bug_state in $(find "$SPECS_DIR" -path "*/bugs/*/state.json" | sort 2>/dev/null); do
  ((bug_total++))
  bug_status="$(grep -oE '"status"[[:space:]]*:[[:space:]]*"[^"]+"' "$bug_state" 2>/dev/null | head -1 | sed -E 's/.*"([^"]+)"$/\1/' || echo "unknown")"
  if [[ "$bug_status" == "done" ]]; then
    ((bug_done++))
  else
    ((bug_open++))
  fi
done

printf "\n${BLUE}═══════════════════════════════════════════════════════════════════════════${NC}\n"
printf "  Specs: ${GREEN}%d done${NC} | ${YELLOW}%d in_progress${NC} | ${RED}%d blocked${NC} | %d not_started | %d total\n" \
  "$done_specs" "$in_progress_specs" "$blocked_specs" "$not_started_specs" "$total_specs"

if [[ "$bug_total" -gt 0 ]]; then
  printf "  Bugs:  ${GREEN}%d fixed${NC} | ${RED}%d open${NC} | %d total\n" "$bug_done" "$bug_open" "$bug_total"
fi

if [[ "$total_specs" -gt 0 ]]; then
  pct=$((done_specs * 100 / total_specs))
  printf "  Progress: %d%%\n" "$pct"
  if [[ "$pct" -eq 100 ]]; then
    fun_message milestone_reached
  elif [[ "$pct" -ge 75 ]]; then
    fun_message scope_ready
  fi
fi
printf "${BLUE}═══════════════════════════════════════════════════════════════════════════${NC}\n\n"
