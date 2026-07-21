#!/usr/bin/env bash
# Hermetic selftest for the rapid-tool-delivery workflow mode (IMP-100 Phase 1).
#
# Proves the registered mode is risk-proportional WITHOUT shedding integrity:
#   1. It resolves (exit 0) with a `done` status ceiling.
#   2. It inherits the FULL delivery gate baseline + anti-fabrication contract
#      (universal gates present; per-DoD raw evidence, tests-for-all-scenarios,
#      implementation reality scan, no-defaults/fallbacks all TRUE).
#   3. It relaxes ONLY the heavyweight mandatory planning chain
#      (requireCanonicalPlanningChain: false) and runs a shorter phase order.
#   4. It carries a non-null session budget (Gate G128 dimensions).
#   5. risk-tier-resolve.sh routes a low-risk build-free surface to this mode and
#      ANY high-risk trigger to full-delivery (no self-label bypass), so the mode
#      is only ever selected for genuinely low-risk work.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOLVER="$SCRIPT_DIR/mode-resolver.sh"
RISK="$SCRIPT_DIR/risk-tier-resolve.sh"

FAILURES=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; FAILURES=$((FAILURES + 1)); }

for required in "$RESOLVER" "$RISK"; do
  if [[ ! -x "$required" ]] && [[ ! -f "$required" ]]; then
    echo "rapid-tool-delivery-mode-selftest: required script missing: $required" >&2
    exit 2
  fi
done

if ! command -v yq >/dev/null 2>&1; then
  echo "rapid-tool-delivery-mode-selftest: SKIP (yq not installed)"
  exit 0
fi

echo "Running rapid-tool-delivery-mode selftest..."

resolved=""
if resolved="$(bash "$RESOLVER" --grandfather rapid-tool-delivery 2>/dev/null)"; then
  pass "rapid-tool-delivery resolves (exit 0)"
else
  fail "rapid-tool-delivery did not resolve"
  echo "rapid-tool-delivery-mode-selftest FAILED"
  exit 1
fi

# has_line <label> <regex>
has_line() {
  local label="$1" re="$2"
  if printf '%s\n' "$resolved" | grep -qE "$re"; then
    pass "$label"
  else
    fail "$label (pattern not found: $re)"
  fi
}

# --- (1) done status ceiling ---
has_line "status ceiling is done" '^statusCeiling: done'

# --- (2) full integrity contract inherited ---
has_line "universal gate G001 present" 'requiredGates:.*\bG001\b'
has_line "regression gate G044 present" 'requiredGates:.*\bG044\b'
has_line "deferral gate G040 present" 'requiredGates:.*\bG040\b'
has_line "env-dependency gate G051 present" 'requiredGates:.*\bG051\b'
has_line "protected-regression gate G094 present" 'requiredGates:.*\bG094\b'
has_line "anti-fabrication detection preserved" '^[[:space:]]+antiFabricationDetection: true'
has_line "per-DoD-item raw evidence preserved" '^[[:space:]]+requirePerDodItemRawEvidence: true'
has_line "tests for all real scenarios preserved" '^[[:space:]]+requireTestsForAllRealScenarios: true'
has_line "implementation reality scan preserved" '^[[:space:]]+requireImplementationRealityScan: true'
has_line "no defaults / no fallbacks preserved" '^[[:space:]]+requireNoDefaultsNoFallbacks: true'
has_line "integration completeness preserved" '^[[:space:]]+requireIntegrationCompleteness: true'

# --- (3) planning chain relaxed + shorter phase order ---
has_line "mandatory planning chain relaxed" '^[[:space:]]+requireCanonicalPlanningChain: false'
has_line "low riskClass declared" '^[[:space:]]+riskClass: low'
has_line "build-free constraint declared" '^[[:space:]]+requireBuildFree: true'
has_line "high-risk escalation constraint declared" '^[[:space:]]+forceFullDeliveryOnHighRisk: true'
has_line "phase order is the shorter delivery lane" '^phaseOrder: \[select, implement, test, validate, docs, finalize\]'

# --- (4) non-null session budget (G128 dimensions) ---
has_line "session budget iterations cap = 2" '^[[:space:]]+maxTotalConvergenceIterations: 2'
has_line "session budget wall-clock cap = 90" '^[[:space:]]+maxWallClockMinutes: 90'
has_line "session budget tool-call cap = 250" '^[[:space:]]+maxToolCalls: 250'

# --- (5) risk-tier-resolve routes low-risk here, high-risk to full-delivery ---
low_out="$(bash "$RISK" --surface "Add a build-free single-file static HTML tool, no backend" 2>&1)"
if printf '%s\n' "$low_out" | grep -qx "tier=rapid-tool-delivery"; then
  pass "risk-tier-resolve routes a low-risk build-free surface to rapid-tool-delivery"
else
  fail "low-risk surface did not route to rapid-tool-delivery (got: $(printf '%s' "$low_out" | grep '^tier=' || true))"
fi

high_out="$(bash "$RISK" --surface "self-contained html tool with jwt auth" 2>&1)"
if printf '%s\n' "$high_out" | grep -qx "tier=full-delivery"; then
  pass "risk-tier-resolve escalates a high-risk trigger to full-delivery (no self-label bypass)"
else
  fail "high-risk surface did not escalate to full-delivery (got: $(printf '%s' "$high_out" | grep '^tier=' || true))"
fi

# --- (6) R4 fast-lane terminal: delivered_fast is a recognized terminal-for-mode ---
IS_TERMINAL="$SCRIPT_DIR/is-terminal-for-mode.sh"
if printf '%s\n' "$resolved" | yq -e '(.terminalAliases // []) | contains(["delivered_fast"])' >/dev/null 2>&1; then
  pass "declares delivered_fast as a terminal alias (R4 fast-lane terminal)"
else
  fail "rapid-tool-delivery should declare terminalAliases: [ delivered_fast ]"
fi
if bash "$IS_TERMINAL" delivered_fast rapid-tool-delivery >/dev/null 2>&1; then
  pass "is-terminal-for-mode recognizes delivered_fast as terminal-for-mode"
else
  fail "is-terminal-for-mode should recognize delivered_fast for rapid-tool-delivery"
fi
if bash "$IS_TERMINAL" "done" rapid-tool-delivery >/dev/null 2>&1; then
  pass "is-terminal-for-mode still recognizes the done ceiling"
else
  fail "is-terminal-for-mode should still recognize done for rapid-tool-delivery"
fi
if ! bash "$IS_TERMINAL" in_progress rapid-tool-delivery >/dev/null 2>&1; then
  pass "is-terminal-for-mode treats in_progress as non-terminal"
else
  fail "is-terminal-for-mode should NOT treat in_progress as terminal for rapid-tool-delivery"
fi

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "rapid-tool-delivery-mode-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "rapid-tool-delivery-mode-selftest: all cases passed."
