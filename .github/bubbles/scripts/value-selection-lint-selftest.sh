#!/usr/bin/env bash
# value-selection-lint-selftest.sh
#
# Hermetic selftest for value-selection-lint.sh.
#
# Stages temp markdown files and invokes the lint. Asserts:
#   - A markdown file that contains a complete "Value-First Selection
#     Cycle" block (header, scoring table, tie-breaker line, selected
#     item line, selected workflow mode line, rationale line) passes
#     (exit 0) with PASS in the output.
#   - A markdown file with no "Value-First Selection Cycle" header
#     fails (exit non-zero) and surfaces the missing-cycle message.
#
# Cleans up the temp tree on exit via trap.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINT="$SCRIPT_DIR/value-selection-lint.sh"

if [[ ! -f "$LINT" ]]; then
  echo "[selftest value-selection-lint] FAIL: target script missing at $LINT" >&2
  exit 1
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

failures=0
pass() { echo "  PASS: $1"; }
fail() { echo "  FAIL: $1"; failures=$((failures + 1)); }

# --- Case 1: file with no Value-First Selection Cycle → exit non-zero ---
missing="$TMPDIR/no-cycle.md"
cat > "$missing" <<'EOF'
# Notes

Some narrative paragraphs without any value-selection block.
EOF

echo "[selftest value-selection-lint] Case 1: missing cycle → exit non-zero"
log1="$TMPDIR/log1.txt"
set +e
bash "$LINT" "$missing" >"$log1" 2>&1
status1=$?
set -e
if [[ "$status1" -ne 0 ]]; then
  pass "missing-cycle file exits non-zero (got $status1)"
else
  fail "missing-cycle file should exit non-zero (got $status1)"
  sed -n '1,40p' "$log1"
fi
if grep -Fq "No 'Value-First Selection Cycle' sections found" "$log1"; then
  pass "output reports 'No Value-First Selection Cycle sections found'"
else
  fail "expected missing-cycle message in output"
  sed -n '1,40p' "$log1"
fi

# --- Case 2: file with a complete Value-First Selection Cycle → exit 0 ---
complete="$TMPDIR/with-cycle.md"
cat > "$complete" <<'EOF'
# Sprint Notes

Some narrative.

#### Value-First Selection Cycle <selftest>

| Rank | Candidate | Type | userImpact (0-5) | deliveryBlocker (0-5) | complianceRisk (0-5) | regressionRisk (0-5) | readiness (0-5) | effortInverse (0-5) | Weighted Score (0-100) |
| ---- | --------- | ---- | ---------------- | --------------------- | -------------------- | -------------------- | --------------- | ------------------- | ---------------------- |
| 1    | Scope A   | feat | 5                | 5                     | 0                    | 0                    | 5               | 4                   | 92                     |
| 2    | Scope B   | bug  | 3                | 2                     | 0                    | 1                    | 4               | 5                   | 67                     |

- Tie-breaker applied: none required (clear delta between rank 1 and rank 2).
- Selected item: Scope A
- Selected workflow mode: full-delivery
- Why highest value now: Scope A unblocks downstream consumers and has the highest user impact in the queue.
EOF

echo "[selftest value-selection-lint] Case 2: complete cycle → exit 0"
log2="$TMPDIR/log2.txt"
set +e
bash "$LINT" "$complete" >"$log2" 2>&1
status2=$?
set -e
if [[ "$status2" -eq 0 ]]; then
  pass "complete-cycle file exits 0 (got $status2)"
else
  fail "complete-cycle file should exit 0 (got $status2)"
  sed -n '1,80p' "$log2"
fi
if grep -Fq 'Value selection lint PASSED.' "$log2"; then
  pass "output reports 'Value selection lint PASSED.'"
else
  fail "expected 'Value selection lint PASSED.' in output"
  sed -n '1,80p' "$log2"
fi

if [[ "$failures" -eq 0 ]]; then
  echo "[selftest value-selection-lint] PASS"
  exit 0
fi

echo "[selftest value-selection-lint] FAIL: $failures assertion(s)"
exit 1
