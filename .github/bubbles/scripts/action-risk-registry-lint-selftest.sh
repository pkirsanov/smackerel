#!/usr/bin/env bash
# action-risk-registry-lint-selftest.sh — hermetic coverage for Gate G139.
#
# Proves the action risk registry lint can actually FAIL.
#
# IMP-052 ARR-1/ARR-2/ARR-7: before this selftest the lint validated only 9 of
# the 39 registered commands and its final whole-file grep was an inert
# alternation, so a bogus risk class could be introduced for 30 commands with no
# objection. The lint runs live in framework-validate, so it was trusted in the
# tier without any proof it could fail. Every scenario below therefore asserts a
# NON-zero exit for a registry that must be rejected, plus a clean-baseline
# control so a lint that rejects everything cannot pass this suite either.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINT="$SCRIPT_DIR/action-risk-registry-lint.sh"
if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  FRAMEWORK_DIR="$(cd "$SCRIPT_DIR/../../.." && pwd)/.github/bubbles"
else
  FRAMEWORK_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)/bubbles"
fi
REAL_REGISTRY="$FRAMEWORK_DIR/action-risk-registry.yaml"

PASS=0
FAIL=0
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

ok() {
  PASS=$((PASS + 1))
  printf 'ok   %s\n' "$1"
}
no() {
  FAIL=$((FAIL + 1))
  printf 'FAIL: %s\n' "$1"
}

# Run the lint against a candidate registry, echo its exit code.
run_lint() {
  BUBBLES_ACTION_RISK_REGISTRY="$1" bash "$LINT" >/dev/null 2>&1
  printf '%s' "$?"
}

assert_exit() {
  local label="$1" want="$2" got="$3"
  if [[ "$got" == "$want" ]]; then
    ok "$label (exit $got)"
  else
    no "$label — expected exit $want, got $got"
  fi
}

if [[ ! -f "$REAL_REGISTRY" ]]; then
  no "S0 real registry present at $REAL_REGISTRY"
  printf '\nPASS=%s FAIL=%s\n' "$PASS" "$FAIL"
  exit 1
fi

# --- S1: the shipped registry is clean (control) ----------------------------
# Without this a lint that rejected everything would score a perfect suite.
cp "$REAL_REGISTRY" "$WORK/s1.yaml"
assert_exit "S1 shipped registry passes" 0 "$(run_lint "$WORK/s1.yaml")"

# --- S2: invalid class on a REQUIRED command --------------------------------
awk '/^  doctor:$/ { f = 1 }
     f && /defaultRiskClass:/ { sub(/read_only|owned_mutation/, "bogus_class"); f = 0 }
     { print }' "$REAL_REGISTRY" > "$WORK/s2.yaml"
assert_exit "S2 invalid class on required command rejected" 1 "$(run_lint "$WORK/s2.yaml")"

# --- S3: invalid class on a command the OLD lint never validated ------------
awk '/^  status:$/ { f = 1 }
     f && /defaultRiskClass:/ { sub(/read_only|owned_mutation/, "bogus_class"); f = 0 }
     { print }' "$REAL_REGISTRY" > "$WORK/s3.yaml"
assert_exit "S3 invalid class on previously unvalidated command rejected" 1 "$(run_lint "$WORK/s3.yaml")"

# --- S4: a near-miss typo must not be treated as the real class -------------
# This is the exact shape that reached the gate's silent allow.
awk '/^  upgrade:$/ { f = 1 }
     f && /defaultRiskClass:/ { sub(/read_only|owned_mutation/, "destructive_mutuation"); f = 0 }
     { print }' "$REAL_REGISTRY" > "$WORK/s4.yaml"
assert_exit "S4 typo class rejected" 1 "$(run_lint "$WORK/s4.yaml")"

# --- S5: a command with NO defaultRiskClass at all --------------------------
awk '/^  status:$/ { f = 1; print; next }
     f && /defaultRiskClass:/ { f = 0; next }
     { print }' "$REAL_REGISTRY" > "$WORK/s5.yaml"
assert_exit "S5 missing defaultRiskClass rejected" 1 "$(run_lint "$WORK/s5.yaml")"

# --- S6: an override value must be validated too ----------------------------
awk '{ if ($0 == "      sync: owned_mutation") print "      sync: bogus_override"; else print }' \
  "$REAL_REGISTRY" > "$WORK/s6.yaml"
assert_exit "S6 invalid override value rejected" 1 "$(run_lint "$WORK/s6.yaml")"

# --- S7: registry vocabulary drifting from the shared library ---------------
awk '{ if ($0 == "- runtime_teardown") print "- renamed_class"; else print }' \
  "$REAL_REGISTRY" > "$WORK/s7.yaml"
assert_exit "S7 registry validRiskClasses drift rejected" 1 "$(run_lint "$WORK/s7.yaml")"

# --- S8: recall's fail-safe default must be preserved -----------------------
# recall's unknown-operation default is deliberately owned_mutation, not
# read_only; weakening it would silently downgrade every unmapped operation.
awk '/^  recall:$/ { f = 1 }
     f && /defaultRiskClass:/ { sub(/owned_mutation/, "read_only"); f = 0 }
     { print }' "$REAL_REGISTRY" > "$WORK/s8.yaml"
assert_exit "S8 weakened recall default rejected" 1 "$(run_lint "$WORK/s8.yaml")"

# --- S9: a required command removed entirely --------------------------------
awk '/^  policy:$/ { skip = 1; next }
     skip && /^  [A-Za-z0-9_-]+:$/ { skip = 0 }
     skip { next }
     { print }' "$REAL_REGISTRY" > "$WORK/s9.yaml"
assert_exit "S9 removed required command rejected" 1 "$(run_lint "$WORK/s9.yaml")"

# --- S10: a registry declaring no commands at all ---------------------------
printf 'version: 1\nvalidRiskClasses:\n- read_only\ncommands:\n' > "$WORK/s10.yaml"
assert_exit "S10 empty command set rejected" 1 "$(run_lint "$WORK/s10.yaml")"

# --- S11: a missing registry file -------------------------------------------
assert_exit "S11 absent registry rejected" 1 "$(run_lint "$WORK/does-not-exist.yaml")"

# --- S12: NO environment variable may suppress a finding --------------------
# A bypass would make every scenario above decorative.
#
# The claim under test is "this variable cannot make the lint PASS", so the
# assertion is exit != 0, NOT exit == 1. Suppression means exit 0 and nothing
# else. An earlier form required exactly 1 and therefore reported "bypass
# present" when the lint died on SIGPIPE (141) under machine load -- a false
# accusation of a security hole, which is the fastest way to teach an operator
# to ignore this suite.
for escape in BUBBLES_SKIP_ACTION_RISK BUBBLES_RISK_SKIP BUBBLES_ACTION_RISK_ALLOW_INVALID \
  SKIP_ACTION_RISK_LINT BUBBLES_RISK_CONFIRM BUBBLES_RISK_BLOCK; do
  got="$(env "$escape=1" BUBBLES_ACTION_RISK_REGISTRY="$WORK/s3.yaml" bash "$LINT" >/dev/null 2>&1; printf '%s' "$?")"
  if [[ "$got" != "0" ]]; then
    ok "S12 $escape cannot suppress a finding (exit $got)"
  else
    no "S12 $escape SUPPRESSED a finding (exit 0) — bypass present"
  fi
done

printf '\nPASS=%s FAIL=%s\n' "$PASS" "$FAIL"
[[ "$FAIL" -eq 0 ]]
