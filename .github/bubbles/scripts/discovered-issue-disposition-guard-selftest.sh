#!/usr/bin/env bash
set -uo pipefail

# discovered-issue-disposition-guard-selftest.sh
#
# Hermetic selftest for `bubbles/scripts/discovered-issue-disposition-guard.sh`
# (Gate G095 — discovered_issue_disposition_gate).
#
# Stages disposable spec-tree fixtures under a `mktemp -d` workspace and
# asserts the exit-code contract for the guard's real behaviors: clean pass,
# forbidden deferral phrases without disposition (BLOCK), inline-artifact
# disposition (pass), today-dated `## Discovered Issues` disposition (pass),
# the RESULT-ENVELOPE scan path, and the malformed-input fail-fast paths.
#
# Scenarios:
#   S0   Missing specDir argument                  → exit 2
#   S0b  Non-existent specDir path                  → exit 2
#   S1   Forbidden phrase, no disposition           → exit 1  (ADVERSARIAL:
#        (report.md "out of scope")                    proves the gate catches
#                                                       an unfiled deferral)
#   S2   Forbidden phrase + inline BUG-NNN citation  → exit 0
#   S3   Forbidden phrase + today-dated `##           → exit 0
#        Discovered Issues` disposition row
#   S3b  Forbidden phrase + ONLY a yesterday-dated    → exit 1  (ADVERSARIAL:
#        Discovered Issues row                          proves the row must be
#                                                       dated today, not stale)
#   S4   Clean report, zero forbidden phrases        → exit 0
#   S5   --envelope points at a non-existent file    → exit 2
#   S6   Envelope narrative carries an unfiled         → exit 1  (envelope scan
#        deferral phrase                                 path)
#
# Reference:
#   bubbles/registry/gates.yaml → G095
#   agents/bubbles_shared/operating-baseline.md → "Discovered-Issue Disposition"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD_SCRIPT="$SCRIPT_DIR/discovered-issue-disposition-guard.sh"

if [[ ! -x "$GUARD_SCRIPT" ]]; then
  echo "selftest: guard script not executable: $GUARD_SCRIPT" >&2
  exit 2
fi

WORKSPACE="$(mktemp -d -t bubbles-g095-selftest-XXXXXXXX)"
trap 'rm -rf "$WORKSPACE"' EXIT INT TERM

TODAY="$(date -u +%Y-%m-%d)"
YESTERDAY="$(date -u -d 'yesterday' +%Y-%m-%d 2>/dev/null || date -u -v-1d +%Y-%m-%d 2>/dev/null || echo "2000-01-01")"

PASS_COUNT=0
FAIL_COUNT=0
FAILED_SCENARIOS=()

pass() {
  echo "  PASS: $1"
  PASS_COUNT=$((PASS_COUNT + 1))
}
bad() {
  echo "  FAIL: $1" >&2
  FAIL_COUNT=$((FAIL_COUNT + 1))
  FAILED_SCENARIOS+=("$1")
}

# Stage a fresh spec dir under the workspace and emit its absolute path.
new_spec_dir() {
  local name="$1"
  local d="$WORKSPACE/$name"
  mkdir -p "$d"
  printf '%s' "$d"
}

# Run the guard, capturing exit code into RC.
run_guard() {
  bash "$GUARD_SCRIPT" "$@" >/dev/null 2>&1
  RC=$?
}

# -----------------------------------------------------------------------
# S0: missing specDir argument → exit 2
# -----------------------------------------------------------------------
run_guard
if [[ "$RC" -eq 2 ]]; then
  pass "S0 missing specDir argument exits 2"
else
  bad "S0 missing specDir expected exit 2, got $RC"
fi

# -----------------------------------------------------------------------
# S0b: non-existent specDir path → exit 2
# -----------------------------------------------------------------------
run_guard "$WORKSPACE/does-not-exist-$$"
if [[ "$RC" -eq 2 ]]; then
  pass "S0b non-existent specDir exits 2"
else
  bad "S0b non-existent specDir expected exit 2, got $RC"
fi

# -----------------------------------------------------------------------
# S1: forbidden phrase, no disposition → exit 1 (ADVERSARIAL)
# -----------------------------------------------------------------------
s1="$(new_spec_dir s1-unfiled-deferral)"
cat >"$s1/report.md" <<EOF
# Report

## Test Evidence

The currency rounding bug is out of scope for this session, so we left it.
EOF
run_guard "$s1"
if [[ "$RC" -eq 1 ]]; then
  pass "S1 unfiled 'out of scope' deferral BLOCKs (exit 1)"
else
  bad "S1 unfiled deferral expected exit 1, got $RC"
fi

# -----------------------------------------------------------------------
# S2: forbidden phrase + inline BUG-NNN citation → exit 0
# -----------------------------------------------------------------------
s2="$(new_spec_dir s2-inline-citation)"
cat >"$s2/report.md" <<EOF
# Report

## Test Evidence

The currency rounding bug is out of scope here; filed as BUG-417 with a repro.
EOF
run_guard "$s2"
if [[ "$RC" -eq 0 ]]; then
  pass "S2 forbidden phrase with inline BUG-NNN citation passes (exit 0)"
else
  bad "S2 inline-citation expected exit 0, got $RC"
fi

# -----------------------------------------------------------------------
# S3: forbidden phrase + today-dated ## Discovered Issues row → exit 0
# -----------------------------------------------------------------------
s3="$(new_spec_dir s3-today-disposition)"
cat >"$s3/report.md" <<EOF
# Report

## Test Evidence

The currency rounding bug is out of scope for this session.

## Discovered Issues

| Observed | Disposition | Reference |
|----------|-------------|-----------|
| $TODAY | bug-filed | BUG-417 |
EOF
run_guard "$s3"
if [[ "$RC" -eq 0 ]]; then
  pass "S3 forbidden phrase with today-dated Discovered Issues row passes (exit 0)"
else
  bad "S3 today-dated disposition expected exit 0, got $RC"
fi

# -----------------------------------------------------------------------
# S3b: forbidden phrase + ONLY a yesterday-dated row → exit 1 (ADVERSARIAL)
# -----------------------------------------------------------------------
s3b="$(new_spec_dir s3b-stale-disposition)"
cat >"$s3b/report.md" <<EOF
# Report

## Test Evidence

The currency rounding bug is out of scope for this session.

## Discovered Issues

| Observed | Disposition | Reference |
|----------|-------------|-----------|
| $YESTERDAY | bug-filed | BUG-417 |
EOF
run_guard "$s3b"
if [[ "$RC" -eq 1 ]]; then
  pass "S3b stale (yesterday-only) disposition row still BLOCKs (exit 1)"
else
  bad "S3b yesterday-only disposition expected exit 1, got $RC"
fi

# -----------------------------------------------------------------------
# S4: clean report, zero forbidden phrases → exit 0
# -----------------------------------------------------------------------
s4="$(new_spec_dir s4-clean)"
cat >"$s4/report.md" <<EOF
# Report

## Test Evidence

All scenarios pass. Coverage is complete and every assertion is green.
EOF
run_guard "$s4"
if [[ "$RC" -eq 0 ]]; then
  pass "S4 clean report passes (exit 0)"
else
  bad "S4 clean report expected exit 0, got $RC"
fi

# -----------------------------------------------------------------------
# S5: --envelope points at a non-existent file → exit 2
# -----------------------------------------------------------------------
s5="$(new_spec_dir s5-bad-envelope)"
cat >"$s5/report.md" <<EOF
# Report

All scenarios pass.
EOF
run_guard "$s5" --envelope "$WORKSPACE/no-such-envelope-$$.json"
if [[ "$RC" -eq 2 ]]; then
  pass "S5 non-existent envelope file exits 2"
else
  bad "S5 non-existent envelope expected exit 2, got $RC"
fi

# -----------------------------------------------------------------------
# S6: envelope narrative carries an unfiled deferral phrase → exit 1
# -----------------------------------------------------------------------
s6="$(new_spec_dir s6-envelope-deferral)"
cat >"$s6/report.md" <<EOF
# Report

All scenarios pass.
EOF
envelope6="$WORKSPACE/s6-envelope.txt"
cat >"$envelope6" <<EOF
RESULT-ENVELOPE narrative: the timezone edge case will fix later once we have a repro.
EOF
run_guard "$s6" --envelope "$envelope6"
if [[ "$RC" -eq 1 ]]; then
  pass "S6 envelope narrative with unfiled deferral BLOCKs (exit 1)"
else
  bad "S6 envelope deferral expected exit 1, got $RC"
fi

# -----------------------------------------------------------------------
# Verdict
# -----------------------------------------------------------------------
echo
echo "============================================================"
echo "  discovered-issue-disposition-guard selftest verdict"
echo "    passed assertions: $PASS_COUNT"
echo "    failed assertions: $FAIL_COUNT"
echo "============================================================"
if [[ "$FAIL_COUNT" -gt 0 ]]; then
  printf '  FAILED: %s\n' "${FAILED_SCENARIOS[@]}" >&2
  echo "discovered-issue-disposition-guard-selftest: FAILED" >&2
  exit 1
fi
echo "discovered-issue-disposition-guard-selftest: PASSED"
exit 0
