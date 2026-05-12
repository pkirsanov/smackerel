#!/usr/bin/env bash
# instruction-budget-lint-selftest.sh
#
# Hermetic selftest for instruction-budget-lint.sh.
#
# Stages a temp agents directory containing synthetic agent files and
# invokes the lint with custom budget thresholds via env vars. Asserts:
#   - An agents dir whose only agent file has a directive count below
#     the hard threshold exits 0.
#   - An agents dir whose agent file exceeds the hard threshold exits
#     non-zero and reports "OVER BUDGET" plus the failing filename.
#
# Cleans up the temp tree on exit via trap.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINT="$SCRIPT_DIR/instruction-budget-lint.sh"

if [[ ! -f "$LINT" ]]; then
  echo "[selftest instruction-budget-lint] FAIL: target script missing at $LINT" >&2
  exit 1
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

failures=0
pass() { echo "  PASS: $1"; }
fail() { echo "  FAIL: $1"; failures=$((failures + 1)); }

# --- Case 1: under-budget agents dir → exit 0 ---
under_dir="$TMPDIR/under"
mkdir -p "$under_dir"
cat > "$under_dir/bubbles.calm.agent.md" <<'EOF'
# bubbles.calm

A small agent description. The agent does one thing and does it well.
This file deliberately keeps strong-directive language to a minimum so
it stays well under any reasonable instruction budget.

When invoked, the agent reads its inputs and writes a structured result.
EOF

echo "[selftest instruction-budget-lint] Case 1: under-budget → exit 0"
log1="$TMPDIR/log1.txt"
set +e
INSTRUCTION_BUDGET_WARN=10 INSTRUCTION_BUDGET_HARD=20 \
  bash "$LINT" "$under_dir" >"$log1" 2>&1
status1=$?
set -e
if [[ "$status1" -eq 0 ]]; then
  pass "under-budget agents dir exits 0 (got $status1)"
else
  fail "under-budget agents dir should exit 0 (got $status1)"
  sed -n '1,80p' "$log1"
fi
if grep -Fq 'OK' "$log1"; then
  pass "output marks under-budget agent as 🟢 OK"
else
  fail "expected '🟢 OK' classification in output"
  sed -n '1,80p' "$log1"
fi

# --- Case 2: over-budget agents dir → exit non-zero ---
over_dir="$TMPDIR/over"
mkdir -p "$over_dir"
{
  echo '# bubbles.heavy'
  echo
  for i in $(seq 1 25); do
    echo "- MUST do thing ${i}: this directive line counts toward the budget."
    echo "- NEVER skip step ${i}: this directive line also counts."
  done
} > "$over_dir/bubbles.heavy.agent.md"

echo "[selftest instruction-budget-lint] Case 2: over-budget → exit non-zero"
log2="$TMPDIR/log2.txt"
set +e
INSTRUCTION_BUDGET_WARN=10 INSTRUCTION_BUDGET_HARD=20 \
  bash "$LINT" "$over_dir" >"$log2" 2>&1
status2=$?
set -e
if [[ "$status2" -ne 0 ]]; then
  pass "over-budget agents dir exits non-zero (got $status2)"
else
  fail "over-budget agents dir should exit non-zero (got $status2)"
  sed -n '1,80p' "$log2"
fi
if grep -Fq 'OVER BUDGET' "$log2"; then
  pass "output flags 'OVER BUDGET' status"
else
  fail "expected 'OVER BUDGET' marker in output"
  sed -n '1,80p' "$log2"
fi
if grep -Fq 'bubbles.heavy.agent.md' "$log2"; then
  pass "output names the offending agent file"
else
  fail "expected 'bubbles.heavy.agent.md' in output"
  sed -n '1,80p' "$log2"
fi

if [[ "$failures" -eq 0 ]]; then
  echo "[selftest instruction-budget-lint] PASS"
  exit 0
fi

echo "[selftest instruction-budget-lint] FAIL: $failures assertion(s)"
exit 1
