#!/usr/bin/env bash
# Hermetic selftest for forecast-eval-check.sh (IMP-100 Phase 6 / IMP-020 S6).
# macOS+WSL portable; jq-gated.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOOL="$SCRIPT_DIR/forecast-eval-check.sh"
FAILURES=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  FAILURES=$((FAILURES + 1))
}
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

if ! command -v jq >/dev/null 2>&1; then
  echo "forecast-eval-check-selftest: SKIP (jq not installed)"
  exit 0
fi

field() { printf '%s' "$1" | jq -r ".$2"; }
mk() { printf '%s\n' "$2" > "$1"; }

VALID='{"predictions":[
  {"id":"p1","predictedAt":"2026-01-01T00:00:00Z","resolvedAt":"2026-02-01T00:00:00Z","predicted":0.7,"actual":1},
  {"id":"p2","predictedAt":"2026-01-01T00:00:00Z","resolvedAt":"2026-02-01T00:00:00Z","predicted":0.2,"actual":0}
]}'
LEAKAGE='{"predictions":[
  {"id":"bad","predictedAt":"2026-02-01T00:00:00Z","resolvedAt":"2026-01-01T00:00:00Z","predicted":0.9,"actual":1}
]}'
PERFECT='{"predictions":[
  {"id":"a","predictedAt":"2026-01-01T00:00:00Z","resolvedAt":"2026-02-01T00:00:00Z","predicted":1,"actual":1},
  {"id":"b","predictedAt":"2026-01-01T00:00:00Z","resolvedAt":"2026-02-01T00:00:00Z","predicted":0,"actual":0}
]}'
WORST='{"predictions":[
  {"id":"w","predictedAt":"2026-01-01T00:00:00Z","resolvedAt":"2026-02-01T00:00:00Z","predicted":0,"actual":1}
]}'
BADSHAPE='{"predictions":[
  {"id":"s","predictedAt":"2026-01-01T00:00:00Z","resolvedAt":"2026-02-01T00:00:00Z","predicted":1.5,"actual":1}
]}'

echo "Running forecast-eval-check selftest..."

# T1: valid forecast, no leakage, Brier ~0.065.
f="$TMP_ROOT/valid.json"
mk "$f" "$VALID"
out="$(bash "$TOOL" --forecast "$f")" && rc=0 || rc=$?
if [[ "$rc" -eq 0 && "$(field "$out" valid)" == "true" && "$(field "$out" leakage)" -eq 0 ]] \
  && printf '%s' "$out" | jq -e '.brierScore > 0.06 and .brierScore < 0.07' >/dev/null; then
  pass "T1 valid forecast → valid=true leakage=0 Brier~0.065 (exit 0)"
else
  fail "T1 unexpected (rc=$rc, out=$out)"
fi

# T2: leakage (predictedAt >= resolvedAt) → valid=false; --strict exits 1.
f="$TMP_ROOT/leak.json"
mk "$f" "$LEAKAGE"
out="$(bash "$TOOL" --forecast "$f")"
if [[ "$(field "$out" valid)" == "false" && "$(field "$out" leakage)" -eq 1 ]]; then
  pass "T2 leakage detected → valid=false leakage=1"
else
  fail "T2 expected leakage=1 valid=false (out=$out)"
fi
bash "$TOOL" --forecast "$f" --strict >/dev/null 2>&1 && rc=0 || rc=$?
if [[ "$rc" -eq 1 ]]; then pass "T2b --strict + leakage → exit 1"; else fail "T2b expected exit 1 (rc=$rc)"; fi

# T3: perfect predictions → Brier 0.
f="$TMP_ROOT/perfect.json"
mk "$f" "$PERFECT"
out="$(bash "$TOOL" --forecast "$f")"
if printf '%s' "$out" | jq -e '.brierScore == 0 and .valid == true' >/dev/null; then
  pass "T3 perfect predictions → Brier 0"
else
  fail "T3 expected Brier 0 (out=$out)"
fi

# T4: worst prediction → Brier 1.
f="$TMP_ROOT/worst.json"
mk "$f" "$WORST"
out="$(bash "$TOOL" --forecast "$f")"
if printf '%s' "$out" | jq -e '.brierScore == 1' >/dev/null; then
  pass "T4 worst prediction → Brier 1"
else
  fail "T4 expected Brier 1 (out=$out)"
fi

# T5: invalid shape (predicted 1.5) → non-strict report valid:false exit 0; strict exit 1.
f="$TMP_ROOT/badshape.json"
mk "$f" "$BADSHAPE"
out="$(bash "$TOOL" --forecast "$f")" && rc=0 || rc=$?
if [[ "$rc" -eq 0 && "$(field "$out" valid)" == "false" ]]; then
  pass "T5 invalid shape → valid=false (exit 0 non-strict)"
else
  fail "T5 expected valid=false exit 0 (rc=$rc, out=$out)"
fi
bash "$TOOL" --forecast "$f" --strict >/dev/null 2>&1 && rc=0 || rc=$?
if [[ "$rc" -eq 1 ]]; then pass "T5b invalid shape + --strict → exit 1"; else fail "T5b expected exit 1 (rc=$rc)"; fi

# T6: empty predictions → runtime error.
f="$TMP_ROOT/empty.json"
mk "$f" '{"predictions":[]}'
bash "$TOOL" --forecast "$f" >/dev/null 2>&1 && rc=0 || rc=$?
if [[ "$rc" -eq 2 ]]; then pass "T6 empty predictions → exit 2"; else fail "T6 expected exit 2 (rc=$rc)"; fi

# T7: missing --forecast → usage error.
bash "$TOOL" >/dev/null 2>&1 && rc=0 || rc=$?
if [[ "$rc" -eq 2 ]]; then pass "T7 missing --forecast → exit 2"; else fail "T7 expected exit 2 (rc=$rc)"; fi

# T8: file not found → runtime error.
bash "$TOOL" --forecast "$TMP_ROOT/nope.json" >/dev/null 2>&1 && rc=0 || rc=$?
if [[ "$rc" -eq 2 ]]; then pass "T8 file not found → exit 2"; else fail "T8 expected exit 2 (rc=$rc)"; fi

# T9: --strict with valid forecast → exit 0.
bash "$TOOL" --forecast "$TMP_ROOT/perfect.json" --strict >/dev/null 2>&1 && rc=0 || rc=$?
if [[ "$rc" -eq 0 ]]; then pass "T9 --strict valid → exit 0"; else fail "T9 expected exit 0 (rc=$rc)"; fi

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "forecast-eval-check-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "forecast-eval-check-selftest: all cases passed."
