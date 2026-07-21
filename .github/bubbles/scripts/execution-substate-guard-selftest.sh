#!/usr/bin/env bash
# Hermetic selftest for execution-substate-guard.sh
# (IMP-100 Phase 2 / IMP-024 SCOPE-3). macOS+WSL portable; jq-gated.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/execution-substate-guard.sh"
FAILURES=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  FAILURES=$((FAILURES + 1))
}
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

if ! command -v jq >/dev/null 2>&1; then
  echo "execution-substate-guard-selftest: SKIP (jq not installed)"
  exit 0
fi

mk() {
  local dir="$1" content="$2"
  mkdir -p "$dir"
  printf '%s\n' "$content" > "$dir/state.json"
}
run() {
  local label="$1" exp="$2" dir="$3"
  local rc=0
  bash "$GUARD" "$dir" >/dev/null 2>&1 && rc=0 || rc=$?
  if [[ "$rc" -eq "$exp" ]]; then pass "$label"; else fail "$label (expected exit $exp, got $rc)"; fi
}

echo "Running execution-substate-guard selftest..."

# T1: no state.json → nothing to check.
d="$TMP_ROOT/t1"
mkdir -p "$d"
run "T1 no state.json → exit 0" 0 "$d"

# T2: normal terminal status, no substate → clean.
d="$TMP_ROOT/t2"
mk "$d" '{ "status": "done", "certification": { "status": "done" } }'
run "T2 normal done status, no substate → exit 0" 0 "$d"

# T3-T5: each valid substate value → clean.
d="$TMP_ROOT/t3"
mk "$d" '{ "status": "in_progress", "execution": { "substate": "implemented" } }'
run "T3 substate implemented → exit 0" 0 "$d"
d="$TMP_ROOT/t4"
mk "$d" '{ "status": "in_progress", "execution": { "substate": "independently_verified" } }'
run "T4 substate independently_verified → exit 0" 0 "$d"
d="$TMP_ROOT/t5"
mk "$d" '{ "status": "in_progress", "execution": { "substate": "needs_reverification" } }'
run "T5 substate needs_reverification → exit 0" 0 "$d"

# T6: invalid substate enum → violation.
d="$TMP_ROOT/t6"
mk "$d" '{ "status": "in_progress", "execution": { "substate": "kinda-done" } }'
run "T6 invalid substate enum → exit 1" 1 "$d"

# T7: substate value leaked into top-level status → namespace violation.
d="$TMP_ROOT/t7"
mk "$d" '{ "status": "implemented" }'
run "T7 status = implemented (namespace leak) → exit 1" 1 "$d"

# T8: substate value leaked into certification.status → namespace violation.
d="$TMP_ROOT/t8"
mk "$d" '{ "certification": { "status": "independently_verified" } }'
run "T8 certification.status = independently_verified (leak) → exit 1" 1 "$d"

# T9: substate not a string → runtime error.
d="$TMP_ROOT/t9"
mk "$d" '{ "execution": { "substate": 3 } }'
run "T9 substate non-string → exit 2" 2 "$d"

# T10: missing feature dir → usage error.
run "T10 missing feature dir → exit 2" 2 "$TMP_ROOT/nope"

# T11: malformed JSON → runtime error.
d="$TMP_ROOT/t11"
mkdir -p "$d"
printf '%s\n' '{ bad' > "$d/state.json"
run "T11 malformed JSON → exit 2" 2 "$d"

# T12: valid substate AND terminal certification coexist (different namespaces) → clean.
d="$TMP_ROOT/t12"
mk "$d" '{ "status": "done", "certification": { "status": "done" }, "execution": { "substate": "independently_verified" } }'
run "T12 substate + done cert coexist → exit 0" 0 "$d"

# T13: substate value leaked into resultEnvelope.outcome → namespace violation.
d="$TMP_ROOT/t13"
mk "$d" '{ "resultEnvelope": { "outcome": "needs_reverification" } }'
run "T13 resultEnvelope.outcome = needs_reverification (leak) → exit 1" 1 "$d"

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "execution-substate-guard-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "execution-substate-guard-selftest: all cases passed."
