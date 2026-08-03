#!/usr/bin/env bash
# open-work-surface-selftest.sh (IMP-033 / SCOPE-6 — gap WIP-1)
#
# Asserts that work carried over from a previous session is SURFACED at the
# only moment enforcement is actually possible: the next session's first
# repository-bound command.
#
# The gap this covers is not "the register is wrong". SCOPE-3 already proves
# the register is right. The gap is that a correct register nobody reads is
# worth exactly as much as no register at all. So the assertions here are about
# CONSUMPTION:
#
#   (a) `doctor` prints an Open Work section containing every residue row id
#       from the register. Residue is checked specifically because it is the
#       only row class that exists nowhere else — a derived row can always be
#       rediscovered from its artifact, an unrecorded loose end cannot.
#   (b) `doctor` still exits the way it did before. This is surfacing, not
#       gating. Refusing to start work because the PREVIOUS session left
#       something open would punish the operator for the framework's own gap,
#       and an operator who cannot start work will simply stop running doctor.
#   (c) The `resume-only` mode declares `requireOpenWorkReview`, and the shared
#       module an agent loads to resolve that mode names the command to run.
#       An agent-side obligation cannot be executed by a selftest, so what is
#       asserted is the thing that IS mechanical: the contract exists in both
#       the machine-readable registry and the human-readable module, and the
#       two do not drift apart.
#
# Exit: 0 all cases pass, 1 any case fails.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CLI="$SCRIPT_DIR/cli.sh"
MODES="$REPO_ROOT/bubbles/workflows/modes.yaml"
MODULE="$REPO_ROOT/agents/bubbles_shared/workflow-mode-resolution.md"

TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT

failures=0
pass() { printf 'PASS: %s\n' "$1"; }
fail() { printf 'FAIL: %s\n' "$1"; failures=$((failures + 1)); }

echo "Running open-work surface selftest (IMP-033 SCOPE-6)..."

# ---------------------------------------------------------------------------
# Fixture: a repo whose register carries residue that exists in NO artifact.
# ---------------------------------------------------------------------------
FX="$TMP_ROOT/carried-over"
git init -q "$FX"
git -C "$FX" symbolic-ref HEAD refs/heads/main
mkdir -p "$FX/.specify/memory"
printf '.specify/runtime/\n' > "$FX/.gitignore"

cat > "$FX/.specify/memory/open-work.md" <<'REGISTER'
# Open work

## Residue

| id | title | kind | ref | state | next-owner | next-action | opened | last-seen |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| residue-01 | half-written migration left in the tree | residue | db/migrate.sql | open | framework maintainer | finish it or delete it | 2026-08-01 | 2026-08-01 |
| residue-02 | debug logging added to trace a race | residue | src/worker.rs | open | framework maintainer | remove before the next release | 2026-08-01 | 2026-08-01 |
REGISTER

git -C "$FX" add -A
git -C "$FX" -c user.email=t@example.com -c user.name=Test commit -q -m seed

DOCTOR_OUT="$(cd "$FX" && BUBBLES_REPO_ROOT="$FX" bash "$CLI" doctor 2>&1)"
DOCTOR_RC=$?

# ---------------------------------------------------------------------------
# (a) Every residue row id reaches the operator
# ---------------------------------------------------------------------------
if printf '%s' "$DOCTOR_OUT" | grep -q 'Open Work'; then
  pass "a1 doctor prints an Open Work section"
else
  fail "a1 doctor printed no Open Work section"
fi

missing_ids=""
for id in residue-01 residue-02; do
  printf '%s' "$DOCTOR_OUT" | grep -q "$id" || missing_ids="$missing_ids $id"
done
if [[ -z "$missing_ids" ]]; then
  pass "a2 every residue row id from the register appears in doctor's output"
else
  fail "a2 doctor dropped residue row id(s):$missing_ids"
fi

if printf '%s' "$DOCTOR_OUT" | grep -q 'finish it or delete it'; then
  pass "a3 the next-action is printed, so the row is actionable and not just a name"
else
  fail "a3 doctor printed the id without its next-action"
fi

# Residue is listed before derived rows because it is the only class that
# cannot be rediscovered from an artifact if it is missed.
OW_SECTION="$(printf '%s\n' "$DOCTOR_OUT" | sed -n '/Open Work/,/Hook Health/p')"
if printf '%s\n' "$OW_SECTION" | grep -n 'residue-01' | head -1 | grep -q '^[0-9]'; then
  pass "a4 residue rows render inside the Open Work section, not elsewhere in the report"
else
  fail "a4 residue rows did not render inside the Open Work section"
fi

# ---------------------------------------------------------------------------
# (b) Surfacing, never gating
# ---------------------------------------------------------------------------
CLEAN="$TMP_ROOT/nothing-open"
git init -q "$CLEAN"
git -C "$CLEAN" symbolic-ref HEAD refs/heads/main
mkdir -p "$CLEAN/.specify/memory"
printf '.specify/runtime/\n' > "$CLEAN/.gitignore"
cat > "$CLEAN/.specify/memory/open-work.md" <<'REGISTER'
# Open work

## Residue

| id | title | kind | ref | state | next-owner | next-action | opened | last-seen |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
REGISTER
git -C "$CLEAN" add -A
git -C "$CLEAN" -c user.email=t@example.com -c user.name=Test commit -q -m seed

(cd "$CLEAN" && BUBBLES_REPO_ROOT="$CLEAN" bash "$CLI" doctor >/dev/null 2>&1)
CLEAN_RC=$?

if [[ "$DOCTOR_RC" -eq "$CLEAN_RC" ]]; then
  pass "b1 carried-over work does not change doctor's exit code (surfacing, not gating)"
else
  fail "b1 doctor exited $DOCTOR_RC with open work but $CLEAN_RC without — open work is gating"
fi

CLEAN_OUT="$(cd "$CLEAN" && BUBBLES_REPO_ROOT="$CLEAN" bash "$CLI" doctor 2>&1)"
if printf '%s' "$CLEAN_OUT" | grep -q 'Open work: nothing carried over'; then
  pass "b2 an empty register renders as a positive statement, not as silence"
else
  fail "b2 an empty register produced no explicit 'nothing carried over' line"
fi

# A section that only ever says "nothing" would pass b2 while being useless.
# b3 is the negative control: the same code path must NOT claim clean when the
# register holds rows.
if printf '%s' "$DOCTOR_OUT" | grep -q 'Open work: nothing carried over'; then
  fail "b3 doctor claimed nothing was carried over while the register held residue"
else
  pass "b3 doctor does not claim clean when the register holds residue"
fi

# ---------------------------------------------------------------------------
# (c) The resume contract exists in BOTH places and has not drifted
# ---------------------------------------------------------------------------
RESUME_BLOCK="$(sed -n '/^  resume-only:/,/^  [a-z][a-z0-9-]*:$/p' "$MODES")"
if printf '%s\n' "$RESUME_BLOCK" | grep -q 'requireOpenWorkReview: true'; then
  pass "c1 resume-only declares requireOpenWorkReview in the machine-readable registry"
else
  fail "c1 resume-only does not declare requireOpenWorkReview"
fi

if grep -q 'requireOpenWorkReview' "$MODULE"; then
  pass "c2 the shared module an agent loads names the same constraint"
else
  fail "c2 the constraint exists in modes.yaml but nowhere an agent would read it"
fi

if grep -q 'cli.sh open-work' "$MODULE"; then
  pass "c3 the module names the exact command, so the obligation is executable"
else
  fail "c3 the module states an obligation without naming the command that satisfies it"
fi

# The constraint must not have quietly become a gate. `resume-only` exists to
# resume work, and a mode that can refuse to resume is a mode operators route
# around.
if printf '%s\n' "$RESUME_BLOCK" | grep -qE 'requiredGates:.*openWork|blockOnOpenWork'; then
  fail "c4 resume-only turned open-work review into a gate"
else
  pass "c4 open-work review is a constraint on resume-only, not a gate that can refuse it"
fi

echo ""
if [[ "$failures" -eq 0 ]]; then
  echo "open-work-surface-selftest: all 11 cases passed."
  exit 0
fi
echo "open-work-surface-selftest: $failures of 11 cases FAILED."
exit 1
