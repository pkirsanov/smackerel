#!/usr/bin/env bash
# state-consistency-scan-selftest.sh — hermetic coverage for state-consistency-scan.sh
#
# Proves the two finding classes fire, that deliberate holds and clean specs stay
# quiet, and that the scan is advisory (exit 0) even when it reports findings.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCAN="$SCRIPT_DIR/state-consistency-scan.sh"

if [[ ! -f "$SCAN" ]]; then
  printf 'state-consistency-scan-selftest: required surface missing: %s\n' "$SCAN" >&2
  exit 2
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "state-consistency-scan-selftest: SKIP (jq not installed)"
  exit 0
fi

selftest_tmp_base="${TMPDIR:-$HOME/.cache}"
mkdir -p "$selftest_tmp_base"
WORKSPACE="$(mktemp -d "$selftest_tmp_base/bubbles-state-consistency-selftest.XXXXXX")"
cleanup() {
  rm -rf "$WORKSPACE"
}
trap cleanup EXIT INT TERM

PASS_COUNT=0
FAIL_COUNT=0

pass() {
  PASS_COUNT=$((PASS_COUNT + 1))
  printf 'PASS: %s\n' "$1"
}

fail_test() {
  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf 'FAIL: %s\n' "$1" >&2
}

# write_spec <repo-root> <spec-name> <status> <cert-status|-> <scope-status|-> <phase-claims-json> <blocked-reason|->
write_spec() {
  local root="$1" name="$2" status="$3" cert="$4" scope_status="$5" claims="$6" blocked="$7"
  local dir="$root/specs/$name"
  mkdir -p "$dir"

  local cert_block='{}'
  [[ "$cert" == "-" ]] || cert_block="$(jq -cn --arg s "$cert" '{status: $s}')"
  local blocked_json='null'
  [[ "$blocked" == "-" ]] || blocked_json="$(jq -cn --arg b "$blocked" '$b')"

  jq -n \
    --arg status "$status" \
    --argjson certification "$cert_block" \
    --argjson claims "$claims" \
    --argjson blockedReason "$blocked_json" \
    '{version: 3, status: $status, workflowMode: "full-delivery",
      blockedReason: $blockedReason,
      execution: {completedPhaseClaims: $claims},
      certification: $certification}' > "$dir/state.json"

  if [[ "$scope_status" != "-" ]]; then
    printf '# Scopes\n\n## Scope 1\n**Status:** %s\n' "$scope_status" > "$dir/scopes.md"
  fi
}

run_scan() {
  local root="$1" out="$2"
  set +e
  bash "$SCAN" --repo-root "$root" > "$out" 2>&1
  local rc=$?
  set -e
  printf '%s' "$rc"
}

# --- Case 1: no specs/ directory is a clean no-op ---------------------------
empty_root="$WORKSPACE/empty"
mkdir -p "$empty_root"
out="$WORKSPACE/empty.out"
rc="$(run_scan "$empty_root" "$out")"
if [[ "$rc" == "0" ]] && grep -q 'no specs/ directory' "$out"; then
  pass "absent specs/ directory is a clean no-op"
else
  fail_test "absent specs/ directory is a clean no-op (rc=$rc)"
fi

# --- Case 2: clean specs produce zero findings ------------------------------
clean_root="$WORKSPACE/clean"
write_spec "$clean_root" 001-clean in_progress in_progress "In Progress" '[]' -
write_spec "$clean_root" 002-agreeing "done" "done" "Done" '["implement"]' -
out="$WORKSPACE/clean.out"
rc="$(run_scan "$clean_root" "$out")"
if [[ "$rc" == "0" ]] && grep -q 'OK — zero findings' "$out" && ! grep -q 'FINDING' "$out"; then
  pass "agreeing mirrors and claimed work produce zero findings"
else
  fail_test "agreeing mirrors and claimed work produce zero findings (rc=$rc)"
fi

# --- Case 3: mirror divergence is reported ----------------------------------
mirror_root="$WORKSPACE/mirror"
write_spec "$mirror_root" 016-half-written specs_hardened not_started "Not Started" '[]' -
out="$WORKSPACE/mirror.out"
rc="$(run_scan "$mirror_root" "$out")"
if [[ "$rc" == "0" ]] \
  && grep -q 'FINDING: mirror-divergence: specs/016-half-written: status=specs_hardened certification.status=not_started' "$out" \
  && grep -q 'bubbles.validate' "$out"; then
  pass "mirror divergence is reported with both values and the owning agent"
else
  fail_test "mirror divergence is reported with both values and the owning agent (rc=$rc)"
fi

# --- Case 4: under-claim via a Done scope -----------------------------------
underclaim_root="$WORKSPACE/underclaim"
write_spec "$underclaim_root" 006-shipped not_started not_started "Done" '[]' -
out="$WORKSPACE/underclaim.out"
rc="$(run_scan "$underclaim_root" "$out")"
if [[ "$rc" == "0" ]] && grep -q 'FINDING: status-behind-evidence: specs/006-shipped: status=not_started but doneScopes=1' "$out"; then
  pass "not_started with a Done scope is reported as status-behind-evidence"
else
  fail_test "not_started with a Done scope is reported as status-behind-evidence (rc=$rc)"
fi

# --- Case 5: under-claim via completed phase claims -------------------------
phase_root="$WORKSPACE/phases"
write_spec "$phase_root" 016-phases not_started not_started "Not Started" '["docs","harden"]' -
out="$WORKSPACE/phases.out"
rc="$(run_scan "$phase_root" "$out")"
if [[ "$rc" == "0" ]] && grep -q 'completedPhaseClaims=2' "$out"; then
  pass "not_started with completed phase claims is reported"
else
  fail_test "not_started with completed phase claims is reported (rc=$rc)"
fi

# --- Case 6: a stated hold is not silent drift ------------------------------
held_root="$WORKSPACE/held"
write_spec "$held_root" 015-held not_started not_started "Done" '["implement"]' "waiting on an upstream dependency"
out="$WORKSPACE/held.out"
rc="$(run_scan "$held_root" "$out")"
if [[ "$rc" == "0" ]] && ! grep -q 'status-behind-evidence' "$out"; then
  pass "a spec parked with a blockedReason is not flagged as drift"
else
  fail_test "a spec parked with a blockedReason is not flagged as drift (rc=$rc)"
fi

# --- Case 7: not_started with no evidence stays quiet -----------------------
fresh_root="$WORKSPACE/fresh"
write_spec "$fresh_root" 014-fresh not_started not_started "Not Started" '[]' -
out="$WORKSPACE/fresh.out"
rc="$(run_scan "$fresh_root" "$out")"
if [[ "$rc" == "0" ]] && ! grep -q 'FINDING' "$out"; then
  pass "a genuinely unstarted spec produces no finding"
else
  fail_test "a genuinely unstarted spec produces no finding (rc=$rc)"
fi

# --- Case 8: findings never block -------------------------------------------
both_root="$WORKSPACE/both"
write_spec "$both_root" 001-diverged "done" not_started "Done" '[]' -
write_spec "$both_root" 002-behind not_started not_started "Done" '[]' -
out="$WORKSPACE/both.out"
rc="$(run_scan "$both_root" "$out")"
if [[ "$rc" == "0" ]] && grep -q '2 finding(s)' "$out" && grep -q 'advisory, exit 0' "$out"; then
  pass "findings are advisory and never change the exit code"
else
  fail_test "findings are advisory and never change the exit code (rc=$rc)"
fi

# --- Case 9: malformed state.json is another guard's finding ----------------
malformed_root="$WORKSPACE/malformed"
mkdir -p "$malformed_root/specs/003-malformed"
printf '%s\n' '{not-json' > "$malformed_root/specs/003-malformed/state.json"
write_spec "$malformed_root" 004-ok in_progress in_progress "In Progress" '[]' -
out="$WORKSPACE/malformed.out"
rc="$(run_scan "$malformed_root" "$out")"
if [[ "$rc" == "0" ]] && grep -q 'zero findings across 1 spec' "$out"; then
  pass "a malformed state.json is skipped without crashing the scan"
else
  fail_test "a malformed state.json is skipped without crashing the scan (rc=$rc)"
fi

# --- Case 10: no bypass flag exists -----------------------------------------
set +e
bash "$SCAN" --repo-root "$both_root" --force > "$WORKSPACE/flag.out" 2>&1
flag_rc=$?
set -e
if [[ "$flag_rc" == "2" ]]; then
  pass "an unknown bypass-shaped flag is rejected"
else
  fail_test "an unknown bypass-shaped flag is rejected (rc=$flag_rc)"
fi

# --- Case 11: an UNCHECKED scaffold picker is not finished work -------------
# The regression. The canonical scope scaffold renders every option unchecked, so a
# counter matching the bare word `Done` reported an untouched template as shipped
# work and flagged every freshly-scaffolded not_started packet.
unchecked_root="$WORKSPACE/unchecked"
write_spec "$unchecked_root" 021-scaffold not_started not_started '[ ] Not started | [ ] In progress | [ ] Done' '[]' -
out="$WORKSPACE/unchecked.out"
rc="$(run_scan "$unchecked_root" "$out")"
if [[ "$rc" == "0" ]] && ! grep -q 'status-behind-evidence' "$out"; then
  pass "an unchecked scaffold picker is not counted as a Done scope"
else
  fail_test "an unchecked scaffold picker is not counted as a Done scope (rc=$rc)"
fi

# --- Case 12: a CHECKED picker is still detected ----------------------------
# Guards Case 11 against being "fixed" by disabling the check outright.
checked_root="$WORKSPACE/checked"
write_spec "$checked_root" 022-shipped not_started not_started '[ ] Not started | [ ] In progress | [x] Done' '[]' -
out="$WORKSPACE/checked.out"
rc="$(run_scan "$checked_root" "$out")"
if [[ "$rc" == "0" ]] && grep -q 'status=not_started but doneScopes=1' "$out"; then
  pass "a checked [x] Done picker is still reported as status-behind-evidence"
else
  fail_test "a checked [x] Done picker is still reported as status-behind-evidence (rc=$rc)"
fi

# --- Case 13: a checked EARLIER option does not imply Done ------------------
inprogress_root="$WORKSPACE/inprogress"
write_spec "$inprogress_root" 023-working not_started not_started '[x] In progress | [ ] Done' '[]' -
out="$WORKSPACE/inprogress.out"
rc="$(run_scan "$inprogress_root" "$out")"
if [[ "$rc" == "0" ]] && ! grep -q 'status-behind-evidence' "$out"; then
  pass "a checked In progress with an unchecked Done is not counted"
else
  fail_test "a checked In progress with an unchecked Done is not counted (rc=$rc)"
fi

printf '\nstate-consistency-scan-selftest: %s passed, %s failed\n' "$PASS_COUNT" "$FAIL_COUNT"
[[ "$FAIL_COUNT" -eq 0 ]] || exit 1
exit 0
