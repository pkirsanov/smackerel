#!/usr/bin/env bash
# Hermetic selftest for assurance-resolve.sh (IMP-100 Phase 2 R4d / Phase 3 R5).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOLVER="$SCRIPT_DIR/assurance-resolve.sh"
FAILURES=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; FAILURES=$((FAILURES + 1)); }

# run_elig <label> <expected-true|false> <resolver-args...>
run_elig() {
  local label="$1" exp="$2"; shift 2
  local out rc=0
  out="$(bash "$RESOLVER" "$@" 2>/dev/null)" && rc=0 || rc=$?
  if [[ "$rc" -eq 0 ]] && printf '%s\n' "$out" | grep -qx "deployEligible=$exp"; then
    pass "$label"
  else
    fail "$label (rc=$rc, got: $(printf '%s' "$out" | grep '^deployEligible=' || echo none))"
  fi
}

# run_floor <label> <expected-minimumAssurance> <resolver-args...>
run_floor() {
  local label="$1" exp="$2"; shift 2
  local out rc=0
  out="$(bash "$RESOLVER" "$@" 2>/dev/null)" && rc=0 || rc=$?
  if [[ "$rc" -eq 0 ]] && printf '%s\n' "$out" | grep -qx "minimumAssurance=$exp"; then
    pass "$label"
  else
    fail "$label (rc=$rc, got: $(printf '%s' "$out" | grep '^minimumAssurance=' || echo none))"
  fi
}

# run_fail <label> <resolver-args...> : expect exit 2.
run_fail() {
  local label="$1"; shift
  local rc=0
  bash "$RESOLVER" "$@" >/dev/null 2>&1 && rc=0 || rc=$?
  if [[ "$rc" -eq 2 ]]; then pass "$label"; else fail "$label (expected exit 2, got $rc)"; fi
}

echo "Running assurance-resolve selftest..."

# ── full always meets the floor ────────────────────────────────────────────
run_elig "T1 full + min full → eligible" true --achieved-level full --minimum-assurance full
run_elig "T2 full + min fast → eligible" true --achieved-level full --minimum-assurance fast

# ── fast meets a fast floor, fails a full floor ────────────────────────────
run_elig "T3 fast + min fast → eligible" true --achieved-level fast --minimum-assurance fast
run_elig "T4 fast + min full → NOT eligible (below floor)" false --achieved-level fast --minimum-assurance full

# ── prototype is NEVER deployable (invariant 1) ────────────────────────────
run_elig "T5 prototype + min fast → NOT eligible" false --achieved-level prototype --minimum-assurance fast
run_elig "T6 prototype + min full → NOT eligible" false --achieved-level prototype --minimum-assurance full

# ── high / unknown risk force the floor to full (defense in depth) ──────────
run_elig "T7 fast + min fast + riskClass high → NOT eligible (escalated to full)" false \
  --achieved-level fast --minimum-assurance fast --risk-class high
run_floor "T7b high-risk escalation surfaces effective floor = full" full \
  --achieved-level fast --minimum-assurance fast --risk-class high
run_elig "T8 fast + min fast + riskClass unknown → NOT eligible (escalated to full)" false \
  --achieved-level fast --minimum-assurance fast --risk-class unknown
run_elig "T9 fast + min fast + riskClass low → eligible (no escalation)" true \
  --achieved-level fast --minimum-assurance fast --risk-class low
run_elig "T10 full + min fast + riskClass high → eligible (full meets escalated full)" true \
  --achieved-level full --minimum-assurance fast --risk-class high

# ── fail-closed: bad input NEVER yields deployEligible=true ─────────────────
run_fail "T11 invalid achieved-level → exit 2" --achieved-level gold --minimum-assurance full
run_fail "T12 minimum-assurance=prototype rejected → exit 2" --achieved-level full --minimum-assurance prototype
run_fail "T13 invalid risk-class → exit 2" --achieved-level fast --minimum-assurance fast --risk-class medium
run_fail "T14 missing achieved-level → exit 2" --minimum-assurance full
run_fail "T15 missing minimum-assurance → exit 2" --achieved-level full
run_fail "T16 unknown flag → exit 2" --achieved-level full --minimum-assurance full --deploy-anyway

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "assurance-resolve-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "assurance-resolve-selftest: all cases passed."
