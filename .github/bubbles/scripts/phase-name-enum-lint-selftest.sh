#!/usr/bin/env bash
# phase-name-enum-lint-selftest.sh (Gate G140: phase_name_enum_integrity_gate) — hermetic coverage for the phase-name enum
# lint (IMP-052 SCOPE-3).
#
# The lint exists because the phase fields were free text while the agent id
# beside them was enum-constrained, so a shipped agent definition could instruct
# writing a phase the registry never registered. Every scenario below therefore
# asserts a NON-zero exit for a tree that must be rejected, plus clean-baseline
# controls so a lint that rejects everything cannot pass this suite either.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINT="$SCRIPT_DIR/phase-name-enum-lint.sh"

PASS=0
FAIL=0
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

ok() { PASS=$((PASS + 1)); printf 'ok   %s\n' "$1"; }
no() { FAIL=$((FAIL + 1)); printf 'FAIL: %s\n' "$1"; }

assert_exit() {
  local label="$1" want="$2" got="$3"
  if [[ "$got" == "$want" ]]; then ok "$label (exit $got)"; else no "$label — expected $want, got $got"; fi
}

# A fixture registry with a known, small phase set.
make_registry() {
  local f="$1"
  mkdir -p "$(dirname "$f")"
  printf 'phases:\n  implement:\n    owner: bubbles.implement\n  test:\n    owner: bubbles.test\n' >"$f"
}

# A fixture tree with one spec state.json.
make_tree() {
  local root="$1" phase="$2"
  mkdir -p "$root/specs/001-x"
  printf '{"execution":{"currentPhase":"%s"}}\n' "$phase" >"$root/specs/001-x/state.json"
}

run_lint() {
  local target="$1" baseline="$2" registry="$3"
  BUBBLES_PHASE_NAME_BASELINE_FILE="$baseline" \
    BUBBLES_WORKFLOWS_FILE="$registry" \
    bash "$LINT" "$target" >/dev/null 2>&1
  printf '%s' "$?"
}

REG="$WORK/registry/workflows.yaml"
make_registry "$REG"
EMPTY_BASELINE="$WORK/empty.baseline"
: >"$EMPTY_BASELINE"

# S1 control: a registered phase passes. Without this a lint that rejects
# everything would score a perfect suite.
T1="$WORK/t1"; make_tree "$T1" "implement"
assert_exit "S1 registered phase passes" 0 "$(run_lint "$T1" "$EMPTY_BASELINE" "$REG")"

# S2: an unregistered phase is rejected.
T2="$WORK/t2"; make_tree "$T2" "totally-made-up-phase"
assert_exit "S2 unregistered phase rejected" 1 "$(run_lint "$T2" "$EMPTY_BASELINE" "$REG")"

# S3: a baselined name is tolerated, so the ratchet lets the lint run at all.
B3="$WORK/b3.baseline"; printf 'totally-made-up-phase\n' >"$B3"
assert_exit "S3 baselined name tolerated" 0 "$(run_lint "$T2" "$B3" "$REG")"

# S4: a name that is neither registered nor baselined still fails even when the
# baseline holds OTHER names — the ratchet must not become a blanket exemption.
B4="$WORK/b4.baseline"; printf 'some-other-name\n' >"$B4"
assert_exit "S4 baseline of a different name does not exempt" 1 "$(run_lint "$T2" "$B4" "$REG")"

# S5: phasesExecuted[] is scanned, not just currentPhase.
T5="$WORK/t5"; mkdir -p "$T5/specs/001-x"
printf '{"executionHistory":[{"phasesExecuted":["implement","ghost-phase"]}]}\n' >"$T5/specs/001-x/state.json"
assert_exit "S5 phasesExecuted[] is scanned" 1 "$(run_lint "$T5" "$EMPTY_BASELINE" "$REG")"

# S6: completedPhaseClaims[] is scanned too.
T6="$WORK/t6"; mkdir -p "$T6/specs/001-x"
printf '{"execution":{"completedPhaseClaims":["implement","phantom-claim"]}}\n' >"$T6/specs/001-x/state.json"
assert_exit "S6 completedPhaseClaims[] is scanned" 1 "$(run_lint "$T6" "$EMPTY_BASELINE" "$REG")"

# S7: declared non-phase values are contract, not debt.
T7="$WORK/t7"; make_tree "$T7" "none"
assert_exit "S7 declared non-phase value tolerated" 0 "$(run_lint "$T7" "$EMPTY_BASELINE" "$REG")"

# S8: a missing registry is a usage failure, never a silent pass.
assert_exit "S8 missing registry refuses" 2 "$(run_lint "$T1" "$EMPTY_BASELINE" "$WORK/nope.yaml")"

# S9: a missing target is a usage failure.
assert_exit "S9 missing target refuses" 2 "$(run_lint "$WORK/no-such-dir" "$EMPTY_BASELINE" "$REG")"

# S10: NO environment variable may suppress a finding. The claim under test is
# "this variable cannot make the lint PASS", so the assertion is exit != 0.
for escape in BUBBLES_SKIP_PHASE_LINT SKIP_PHASE_NAME_LINT BUBBLES_PHASE_ALLOW_UNKNOWN BUBBLES_LINT_SKIP; do
  got="$(env "$escape=1" BUBBLES_PHASE_NAME_BASELINE_FILE="$EMPTY_BASELINE" \
    BUBBLES_WORKFLOWS_FILE="$REG" bash "$LINT" "$T2" >/dev/null 2>&1; printf '%s' "$?")"
  if [[ "$got" != "0" ]]; then ok "S10 $escape cannot suppress a finding (exit $got)"; else no "S10 $escape SUPPRESSED a finding (exit 0) — bypass present"; fi
done

# S11: bypass-shaped flags are refused outright.
for flag in --skip --force --ignore --no-verify --bypass --allow-once; do
  got="$(bash "$LINT" "$T1" "$flag" >/dev/null 2>&1; printf '%s' "$?")"
  if [[ "$got" == "2" ]]; then ok "S11 $flag refused"; else no "S11 $flag not refused (exit $got)"; fi
done

# S12: --update-baseline writes the observed unknowns and then exits clean.
B12="$WORK/b12.baseline"
BUBBLES_PHASE_NAME_BASELINE_FILE="$B12" BUBBLES_WORKFLOWS_FILE="$REG" \
  bash "$LINT" "$T2" --update-baseline >/dev/null 2>&1
if grep -qx 'totally-made-up-phase' "$B12" 2>/dev/null; then
  ok "S12 --update-baseline records the unknown name"
else
  no "S12 --update-baseline did not record the unknown name"
fi
assert_exit "S12 tree is clean after baseline update" 0 "$(run_lint "$T2" "$B12" "$REG")"

printf '\nPASS=%s FAIL=%s\n' "$PASS" "$FAIL"
[[ "$FAIL" -eq 0 ]]
