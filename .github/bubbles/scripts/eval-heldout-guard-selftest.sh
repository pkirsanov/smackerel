#!/usr/bin/env bash
# Hermetic selftest for eval-heldout-guard.sh (IMP-100 Phase 6 / IMP-020 S4).
# macOS+WSL portable; jq-gated.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/eval-heldout-guard.sh"
FAILURES=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  FAILURES=$((FAILURES + 1))
}
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

if ! command -v jq >/dev/null 2>&1; then
  echo "eval-heldout-guard-selftest: SKIP (jq not installed)"
  exit 0
fi

VALID_HELDOUT='{"schemaVersion":2,"taskId":"held-out-001","passThreshold":0.8,"stratum":"bugfix","checks":[{"id":"o","type":"executable-oracle","required":true,"weight":4,"allowedRoot":"oracles","argv":["x.py"]}]}'
VALID_HELDOUT_2='{"schemaVersion":2,"taskId":"held-out-002","passThreshold":0.8,"stratum":"feature","checks":[{"id":"o","type":"semantic-evaluator","required":true,"weight":4}]}'
CONTRACT_GOLDEN='{"schemaVersion":2,"taskId":"golden-001","passThreshold":0.8,"checks":[{"id":"o","type":"executable-oracle","required":true,"weight":4,"allowedRoot":"oracles","argv":["x.py"]}]}'
LEAKED_HELDOUT='{"schemaVersion":2,"taskId":"golden-001","passThreshold":0.8,"checks":[{"id":"o","type":"executable-oracle","required":true,"weight":4,"allowedRoot":"oracles","argv":["x.py"]}]}'
V1_HELDOUT='{"taskId":"held-out-v1","checks":[]}'
NONSUBSTANTIVE_HELDOUT='{"schemaVersion":2,"taskId":"held-out-weak","passThreshold":0.8,"checks":[{"id":"f","type":"file-exists","required":true,"weight":1,"path":"report.md"}]}'

setup_contract() {
  mkdir -p "$1/contract"
  printf '%s\n' "$CONTRACT_GOLDEN" > "$1/contract/golden.json"
}
run() {
  local label="$1" exp="$2"
  shift 2
  local rc=0
  bash "$GUARD" "$@" >/dev/null 2>&1 && rc=0 || rc=$?
  if [[ "$rc" -eq "$exp" ]]; then pass "$label"; else fail "$label (expected exit $exp, got $rc)"; fi
}

echo "Running eval-heldout-guard selftest..."

# T1: no held-out dir → no-op.
d="$TMP_ROOT/t1"
setup_contract "$d"
run "T1 no held-out dir → exit 0" 0 --held-out "$d/held-out" --contract "$d/contract"

# T2: held-out dir empty → no-op.
d="$TMP_ROOT/t2"
setup_contract "$d"
mkdir -p "$d/held-out"
run "T2 empty held-out dir → exit 0" 0 --held-out "$d/held-out" --contract "$d/contract"

# T3: valid held-out (isolated + substantive) → OK.
d="$TMP_ROOT/t3"
setup_contract "$d"
mkdir -p "$d/held-out"
printf '%s\n' "$VALID_HELDOUT" > "$d/held-out/h1.json"
run "T3 valid held-out → exit 0" 0 --held-out "$d/held-out" --contract "$d/contract"

# T4: leaked held-out (taskId in contract) → violation.
d="$TMP_ROOT/t4"
setup_contract "$d"
mkdir -p "$d/held-out"
printf '%s\n' "$LEAKED_HELDOUT" > "$d/held-out/h1.json"
run "T4 leaked held-out id → exit 1" 1 --held-out "$d/held-out" --contract "$d/contract"

# T5: held-out not schemaVersion 2 → violation.
d="$TMP_ROOT/t5"
setup_contract "$d"
mkdir -p "$d/held-out"
printf '%s\n' "$V1_HELDOUT" > "$d/held-out/h1.json"
run "T5 v1 held-out → exit 1" 1 --held-out "$d/held-out" --contract "$d/contract"

# T6: held-out v2 without a required substantive check → violation.
d="$TMP_ROOT/t6"
setup_contract "$d"
mkdir -p "$d/held-out"
printf '%s\n' "$NONSUBSTANTIVE_HELDOUT" > "$d/held-out/h1.json"
run "T6 non-substantive held-out → exit 1" 1 --held-out "$d/held-out" --contract "$d/contract"

# T7: two valid held-out tasks, per-stratum output present → OK, and output shows strata.
d="$TMP_ROOT/t7"
setup_contract "$d"
mkdir -p "$d/held-out"
printf '%s\n' "$VALID_HELDOUT" > "$d/held-out/h1.json"
printf '%s\n' "$VALID_HELDOUT_2" > "$d/held-out/h2.json"
out="$(bash "$GUARD" --held-out "$d/held-out" --contract "$d/contract" 2>&1)" && rc=0 || rc=$?
if [[ "$rc" -eq 0 ]] && printf '%s\n' "$out" | grep -q 'bugfix' && printf '%s\n' "$out" | grep -q 'feature'; then
  pass "T7 two held-out tasks, per-stratum reported → exit 0"
else
  fail "T7 expected per-stratum output (rc=$rc, out=$out)"
fi

# T8: unknown argument → usage error.
run "T8 unknown arg → exit 2" 2 --bogus x

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "eval-heldout-guard-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "eval-heldout-guard-selftest: all cases passed."
