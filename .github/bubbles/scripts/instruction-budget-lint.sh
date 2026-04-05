#!/usr/bin/env bash
# instruction-budget-lint.sh — Measure instruction density in agent prompts
#
# Counts directive lines (MUST, NEVER, REQUIRED, FORBIDDEN, NON-NEGOTIABLE,
# MANDATORY, BLOCKING, ABSOLUTE, imperative verbs at line start) per agent
# prompt file. Reports warning/error when thresholds are exceeded.
#
# Usage:
#   bash bubbles/scripts/instruction-budget-lint.sh [agents_dir]
#
# Arguments:
#   agents_dir  Path to agents directory (default: agents/ relative to repo root)
#
# Exit codes:
#   0 — all agents within budget
#   1 — at least one agent exceeds hard threshold

set -euo pipefail

# --- Configuration ---
WARN_THRESHOLD="${INSTRUCTION_BUDGET_WARN:-120}"
HARD_THRESHOLD="${INSTRUCTION_BUDGET_HARD:-200}"

# --- Resolve paths ---
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
AGENTS_DIR="${1:-$REPO_ROOT/agents}"

if [[ ! -d "$AGENTS_DIR" ]]; then
  echo "ERROR: agents directory not found: $AGENTS_DIR"
  exit 1
fi

# --- Directive pattern ---
# Matches lines containing strong directive language that consumes instruction budget.
# This is intentionally broad — it catches the kinds of instructions that LLMs
# must attend to, not just any mention of the word.
DIRECTIVE_PATTERN='MUST|NEVER|REQUIRED|FORBIDDEN|NON-NEGOTIABLE|MANDATORY|BLOCKING|ABSOLUTE|DO NOT|ALWAYS|SHALL NOT'

# --- Scan ---
exit_code=0
total_files=0
over_warn=0
over_hard=0

printf "\n%-50s %8s %8s %s\n" "Agent" "Lines" "Directives" "Status"
printf "%-50s %8s %8s %s\n" "-----" "-----" "----------" "------"

for agent_file in "$AGENTS_DIR"/bubbles.*.agent.md; do
  [[ -f "$agent_file" ]] || continue
  total_files=$((total_files + 1))

  filename="$(basename "$agent_file")"
  total_lines=$(wc -l < "$agent_file")

  # Count lines containing directive keywords (case-insensitive)
  directive_count=$(grep -ciE "$DIRECTIVE_PATTERN" "$agent_file" 2>/dev/null || true)

  # Classify
  if [[ "$directive_count" -ge "$HARD_THRESHOLD" ]]; then
    status="🔴 OVER BUDGET (>${HARD_THRESHOLD})"
    over_hard=$((over_hard + 1))
    exit_code=1
  elif [[ "$directive_count" -ge "$WARN_THRESHOLD" ]]; then
    status="🟡 WARNING (>${WARN_THRESHOLD})"
    over_warn=$((over_warn + 1))
  else
    status="🟢 OK"
  fi

  printf "%-50s %8d %8d %s\n" "$filename" "$total_lines" "$directive_count" "$status"
done

# --- Also scan shared governance modules ---
SHARED_DIR="$AGENTS_DIR/bubbles_shared"
if [[ -d "$SHARED_DIR" ]]; then
  printf "\n%-50s %8s %8s %s\n" "Shared Module" "Lines" "Directives" "Note"
  printf "%-50s %8s %8s %s\n" "-------------" "-----" "----------" "----"

  for shared_file in "$SHARED_DIR"/*.md; do
    [[ -f "$shared_file" ]] || continue
    filename="$(basename "$shared_file")"
    total_lines=$(wc -l < "$shared_file")
    directive_count=$(grep -ciE "$DIRECTIVE_PATTERN" "$shared_file" 2>/dev/null || true)
    printf "%-50s %8d %8d (loaded by agents)\n" "$filename" "$total_lines" "$directive_count"
  done
fi

# --- Summary ---
printf "\n--- Summary ---\n"
printf "Agent files scanned: %d\n" "$total_files"
printf "Warn threshold: %d directives\n" "$WARN_THRESHOLD"
printf "Hard threshold: %d directives\n" "$HARD_THRESHOLD"
printf "Over warn: %d\n" "$over_warn"
printf "Over hard: %d\n" "$over_hard"

if [[ "$exit_code" -ne 0 ]]; then
  printf "\n⛔ %d agent(s) exceed the hard instruction budget of %d directives.\n" "$over_hard" "$HARD_THRESHOLD"
  printf "   Consider moving control flow from prompt prose into workflows.yaml definitions,\n"
  printf "   shared modules, or scripts. LLMs reliably follow ~150-200 instructions;\n"
  printf "   beyond that, step adherence degrades.\n"
fi

exit "$exit_code"
