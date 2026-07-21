#!/usr/bin/env bash
# Hermetic selftest for assurance-derive.sh (IMP-100 Phase 3 choke point #1 — derivation half).
# Includes composition cases proving assurance-derive.sh + assurance-resolve.sh
# compose (evidence → achieved level → deploy-eligibility).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DERIVE="$SCRIPT_DIR/assurance-derive.sh"
RESOLVE="$SCRIPT_DIR/assurance-resolve.sh"
FAILURES=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; FAILURES=$((FAILURES + 1)); }

# run_field <label> <field> <expected> <derive-args...> : assert stdout line field=expected.
run_field() {
  local label="$1" field="$2" exp="$3"; shift 3
  local out rc=0
  out="$(bash "$DERIVE" "$@" 2>/dev/null)" && rc=0 || rc=$?
  if [[ "$rc" -eq 0 ]] && printf '%s\n' "$out" | grep -qxF "$field=$exp"; then
    pass "$label"
  else
    fail "$label (rc=$rc, got: $(printf '%s' "$out" | grep "^$field=" || echo none))"
  fi
}

# run_fail <label> <derive-args...> : expect exit 2.
run_fail() {
  local label="$1"; shift
  local rc=0
  bash "$DERIVE" "$@" >/dev/null 2>&1 && rc=0 || rc=$?
  if [[ "$rc" -eq 2 ]]; then pass "$label"; else fail "$label (expected exit 2, got $rc)"; fi
}

# run_ok <label> <derive-args...> : expect exit 0.
run_ok() {
  local label="$1"; shift
  local rc=0
  bash "$DERIVE" "$@" >/dev/null 2>&1 && rc=0 || rc=$?
  if [[ "$rc" -eq 0 ]]; then pass "$label"; else fail "$label (expected exit 0, got $rc)"; fi
}

# run_compose <label> <expected-deployEligible> <minimum> <derive-args...> :
# derive the achieved level, feed it into assurance-resolve.sh, assert deployEligible.
run_compose() {
  local label="$1" exp="$2" minimum="$3"; shift 3
  local dout level rc=0
  dout="$(bash "$DERIVE" "$@" 2>/dev/null)" && rc=0 || rc=$?
  if [[ "$rc" -ne 0 ]]; then fail "$label (derive rc=$rc)"; return; fi
  level="$(printf '%s\n' "$dout" | sed -n 's/^achievedLevel=//p')"
  local rout rrc=0
  rout="$(bash "$RESOLVE" --achieved-level "$level" --minimum-assurance "$minimum" 2>/dev/null)" && rrc=0 || rrc=$?
  if [[ "$rrc" -eq 0 ]] && printf '%s\n' "$rout" | grep -qxF "deployEligible=$exp"; then
    pass "$label"
  else
    fail "$label (resolve rc=$rrc for level=$level, got: $(printf '%s' "$rout" | grep '^deployEligible=' || echo none))"
  fi
}

echo "Running assurance-derive selftest..."

# ── full: complete integrity chain incl. audit → done ──────────────────────
run_field "T1 full achievedLevel" achievedLevel full \
  --implement-complete true --tests-complete true --tests-passed true --audit-complete true
run_field "T2 full terminalStatus=done" terminalStatus "done" \
  --implement-complete true --tests-complete true --tests-passed true --audit-complete true
run_field "T3 full missingForFull=none" missingForFull none \
  --implement-complete true --tests-complete true --tests-passed true --audit-complete true

# ── fast: verified but no audit → delivered_fast ───────────────────────────
run_field "T4 fast achievedLevel" achievedLevel fast \
  --implement-complete true --tests-complete true --tests-passed true --audit-complete false
run_field "T5 fast terminalStatus=delivered_fast" terminalStatus delivered_fast \
  --implement-complete true --tests-complete true --tests-passed true --audit-complete false
run_field "T6 fast missingForFull=independent-audit" missingForFull independent-audit \
  --implement-complete true --tests-complete true --tests-passed true --audit-complete false

# ── prototype: failing tests → delivered_prototype ─────────────────────────
run_field "T7 prototype (tests failing) achievedLevel" achievedLevel prototype \
  --implement-complete true --tests-complete true --tests-passed false --audit-complete false
run_field "T8 prototype terminalStatus=delivered_prototype" terminalStatus delivered_prototype \
  --implement-complete true --tests-complete true --tests-passed false --audit-complete false
run_field "T9 prototype (tests failing) missingForFull" missingForFull all-tests-passing,independent-audit \
  --implement-complete true --tests-complete true --tests-passed false --audit-complete false

# ── prototype: incomplete coverage → delivered_prototype ───────────────────
run_field "T10 prototype (coverage incomplete) missingForFull" missingForFull test-coverage-complete,independent-audit \
  --implement-complete true --tests-complete false --tests-passed true --audit-complete false

# ── fail-closed edge: incompleteness derives DOWN even with audit=true ──────
run_field "T11 coverage-incomplete + audit=true → still prototype" achievedLevel prototype \
  --implement-complete true --tests-complete false --tests-passed true --audit-complete true
run_field "T11b that prototype's missingForFull omits audit (audit present)" missingForFull test-coverage-complete \
  --implement-complete true --tests-complete false --tests-passed true --audit-complete true
run_field "T12 tests-failing + audit=true → prototype, missing=all-tests-passing" missingForFull all-tests-passing \
  --implement-complete true --tests-complete true --tests-passed false --audit-complete true

# ── riskClass pass-through (does NOT change the derived level) ──────────────
run_field "T13 riskClass=high passthrough (level unchanged=full)" riskClass high \
  --implement-complete true --tests-complete true --tests-passed true --audit-complete true --risk-class high
run_field "T13b high risk does not lower a full achievement" achievedLevel full \
  --implement-complete true --tests-complete true --tests-passed true --audit-complete true --risk-class high
run_field "T14 riskClass defaults to unknown when omitted" riskClass unknown \
  --implement-complete true --tests-complete true --tests-passed true --audit-complete true

# ── fail-closed: bad input NEVER yields an achievedLevel ───────────────────
run_fail "T15 missing required flag (--audit-complete) → exit 2" \
  --implement-complete true --tests-complete true --tests-passed true
run_fail "T16 implement-complete=false (no delivery) → exit 2" \
  --implement-complete false --tests-complete true --tests-passed true --audit-complete true
run_fail "T17 invalid bool → exit 2" \
  --implement-complete true --tests-complete true --tests-passed maybe --audit-complete true
run_fail "T18 invalid risk-class → exit 2" \
  --implement-complete true --tests-complete true --tests-passed true --audit-complete true --risk-class medium
run_fail "T19 unknown flag → exit 2" \
  --implement-complete true --tests-complete true --tests-passed true --audit-complete true --ship-it
run_ok "T20 --help → exit 0" --help

# ── composition: derive → assurance-resolve deploy-eligibility ─────────────
run_compose "T21 fast achievement under a full floor → NOT deployable" false full \
  --implement-complete true --tests-complete true --tests-passed true --audit-complete false
run_compose "T22 full achievement under a fast floor → deployable" true fast \
  --implement-complete true --tests-complete true --tests-passed true --audit-complete true
run_compose "T23 prototype achievement under a fast floor → NOT deployable" false fast \
  --implement-complete true --tests-complete true --tests-passed false --audit-complete false

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "assurance-derive-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "assurance-derive-selftest: all cases passed."
