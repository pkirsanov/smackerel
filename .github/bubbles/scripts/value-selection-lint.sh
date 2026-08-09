#!/usr/bin/env bash
set -euo pipefail

# Source fun mode support
source "$(dirname "${BASH_SOURCE[0]}")/fun-mode.sh"

target_file="${1:-}"

if [[ -z "$target_file" ]]; then
  echo "ERROR: missing markdown file argument"
  echo "Usage: bash bubbles/scripts/value-selection-lint.sh <markdown-file>"
  exit 2
fi

if [[ ! -f "$target_file" ]]; then
  echo "ERROR: file not found: $target_file"
  exit 2
fi

failures=0
cycles_found=0

fail() {
  local message="$1"
  echo "❌ $message"
  fun_fail
  failures=$((failures + 1))
}

pass() {
  local message="$1"
  echo "✅ $message"
}

cycle_lines="$(grep -nE '^#### Value-First Selection Cycle( |<)' "$target_file" || true)"
if [[ -z "$cycle_lines" ]]; then
  fail "No 'Value-First Selection Cycle' sections found"
else
  cycles_found=$(echo "$cycle_lines" | wc -l | tr -d ' ')
  pass "Found $cycles_found Value-First Selection Cycle section(s)"
fi

if (( cycles_found > 0 )); then
  while IFS= read -r cycle_line; do
    line_number="${cycle_line%%:*}"

    window="$(awk -v start="$line_number" 'NR >= start && NR <= start + 30 { print }' "$target_file")"

    if grep -Fq '| Rank | Candidate | Type | userImpact (0-5) | deliveryBlocker (0-5) | complianceRisk (0-5) | regressionRisk (0-5) | readiness (0-5) | effortInverse (0-5) | Weighted Score (0-100) |' <<< "$window"; then
      pass "Cycle at line $line_number contains required table header"
    else
      fail "Cycle at line $line_number missing required table header"
    fi

    if grep -Eq '^- Tie-breaker applied: ' <<< "$window"; then
      pass "Cycle at line $line_number includes tie-breaker line"
    else
      fail "Cycle at line $line_number missing '- Tie-breaker applied:' line"
    fi

    if grep -Eq '^- Selected item: ' <<< "$window"; then
      pass "Cycle at line $line_number includes selected item line"
    else
      fail "Cycle at line $line_number missing '- Selected item:' line"
    fi

    if grep -Eq '^- Selected workflow mode: ' <<< "$window"; then
      pass "Cycle at line $line_number includes selected workflow mode line"
    else
      fail "Cycle at line $line_number missing '- Selected workflow mode:' line"
    fi

    if grep -Eq '^- Why highest value now: ' <<< "$window"; then
      pass "Cycle at line $line_number includes rationale line"
    else
      fail "Cycle at line $line_number missing '- Why highest value now:' line"
    fi
  done <<< "$cycle_lines"
fi

if (( failures > 0 )); then
  echo ""
  echo "Value selection lint FAILED with $failures issue(s)."
  fun_message lint_dirty
  exit 1
fi

echo ""
echo "Value selection lint PASSED."
fun_message lint_clean
