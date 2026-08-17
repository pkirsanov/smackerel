#!/usr/bin/env bash
# phase-coordinator-selftest.sh — IMP-047 S-C (AC8, AC9, AC10, AC11, AC12).
#
# Every assertion here runs the SHIPPING coordinator against a real interrupted
# process fixture. The phases execute REAL commands and the outcomes are read
# back from the cursor the coordinator wrote, because a coordinator that is only
# ever asked what it would do has never been shown to do it.
#
# Covered:
#   AC8   repeated phases get DISTINCT occurrence ids; resume starts at the
#         FIRST UNRESOLVED occurrence
#   AC9   accepted phase results are NOT replayed
#   AC10  phase relevance drives ACTUAL execution through this consumer
#   AC11  a failed prerequisite marks dependents BLOCKED_NOT_RUN while
#         INDEPENDENT checks still execute and report real diagnostics
#   AC12  each iteration records plan fidelity, and iteration exhaustion never
#         reports success
#
# Hermetic: fixtures live under mktemp and are removed on exit.
#
# Exit codes:
#   0 = all assertions passed
#   1 = at least one assertion failed
#   2 = the coordinator or a dependency is unavailable

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COORDINATOR="$SCRIPT_DIR/phase-coordinator.sh"
NAME="phase-coordinator-selftest"

passes=0
failures=0
pass() {
  passes=$((passes + 1))
  printf 'PASS: %s\n' "$1"
}
fail() {
  failures=$((failures + 1))
  printf 'FAIL: %s\n' "$1"
}

[[ -f "$COORDINATOR" ]] || {
  printf '%s: coordinator not found: %s\n' "$NAME" "$COORDINATOR" >&2
  exit 2
}
command -v python3 >/dev/null 2>&1 || {
  printf '%s: python3 is required\n' "$NAME" >&2
  exit 2
}

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-phase-coordinator.XXXXXX")"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

SPEC="$TMP_DIR/spec"
mkdir -p "$SPEC"
printf '# fixture spec\n' > "$SPEC/spec.md"

# Each phase appends its occurrence id to this file when it actually runs. It is
# the ground truth for "did this execute", separate from what the coordinator
# reports about itself.
RAN="$TMP_DIR/ran.log"
: > "$RAN"
RUN_CMD="printf '%s\n' \"\$BUBBLES_PHASE_OCCURRENCE\" >> $RAN"
FAIL_CMD="printf '%s\n' \"\$BUBBLES_PHASE_OCCURRENCE\" >> $RAN; exit 3"

jget() { printf '%s' "$1" | python3 -c "import json,sys;d=json.load(sys.stdin);print($2)" 2>/dev/null || printf 'ERR'; }
# grep -c prints 0 AND exits 1 on no-match, so a `|| printf 0` fallback would
# emit "0\n0" and break the arithmetic comparison it feeds.
ran_count() { grep -cFx "$1" "$RAN" 2>/dev/null | head -n 1 || true; }

# ---------------------------------------------------------------------------
# AC8 — a mode with `validate` TWICE. The two runs must be distinguishable.
# ---------------------------------------------------------------------------
CURSOR="$TMP_DIR/cursor.json"
plan_all() {
  bash "$COORDINATOR" --spec-dir "$SPEC" --cursor "$CURSOR" --format json --max-iterations 3 \
    --phase "validate=$RUN_CMD" \
    --phase "implement=$1" \
    --phase "validate=$RUN_CMD" 2>&1
}

out="$(bash "$COORDINATOR" --spec-dir "$SPEC" --cursor "$CURSOR" --format json --dry-run \
  --phase "validate=$RUN_CMD" --phase "implement=$RUN_CMD" --phase "validate=$RUN_CMD" 2>&1)"
occ_ids="$(jget "$out" '" ".join(o["occurrenceId"] for o in d["occurrences"])')"
if [[ "$occ_ids" == "validate#1 implement#1 validate#2" ]]; then
  pass "AC8: repeated phases get DISTINCT occurrence ids ($occ_ids)"
else
  fail "AC8: expected distinct occurrence ids, observed [$occ_ids]"
fi

# ---------------------------------------------------------------------------
# AC11 — the INTERRUPTION. `implement` fails, so the dependent `validate#2` is
# BLOCKED_NOT_RUN while the independent check still executes.
# ---------------------------------------------------------------------------
: > "$RAN"
out="$(bash "$COORDINATOR" --spec-dir "$SPEC" --cursor "$CURSOR" --format json --max-iterations 3 \
  --phase "validate=$RUN_CMD" \
  --phase "implement=$FAIL_CMD" \
  --phase "validate=$RUN_CMD" \
  --independent "lint=$RUN_CMD" 2>&1)"
rc=$?
outcome_of() { jget "$out" "[o['outcome'] for o in d['occurrences'] if o['occurrenceId']=='$1'][0]"; }

if [[ "$(outcome_of 'validate#2')" == "BLOCKED_NOT_RUN" ]]; then
  pass "AC11: a dependent of a failed prerequisite is BLOCKED_NOT_RUN"
else
  fail "AC11: expected validate#2 BLOCKED_NOT_RUN, observed $(outcome_of 'validate#2')"
fi
if [[ "$(ran_count 'validate#2')" -eq 0 ]]; then
  pass "AC11: a BLOCKED_NOT_RUN dependent was genuinely NOT executed"
else
  fail "AC11: validate#2 executed despite being reported BLOCKED_NOT_RUN"
fi
if [[ "$(outcome_of 'lint#1')" == "RAN_PASS" && "$(ran_count 'lint#1')" -eq 1 ]]; then
  pass "AC11: an INDEPENDENT check still executed after the prerequisite failed"
else
  fail "AC11: the independent check did not execute (outcome $(outcome_of 'lint#1'), runs $(ran_count 'lint#1'))"
fi
if [[ "$(outcome_of 'implement#1')" == "RAN_FAIL" ]] &&
  [[ "$(jget "$out" "[o['exitCode'] for o in d['occurrences'] if o['occurrenceId']=='implement#1'][0]")" == "3" ]]; then
  pass "AC11: the failed prerequisite reports its REAL diagnostic exit code (3)"
else
  fail "AC11: the failed prerequisite did not report a real exit code"
fi
if [[ "$rc" -ne 0 ]]; then
  pass "AC11: an incomplete run exits non-zero"
else
  fail "AC11: an incomplete run exited 0"
fi

# ---------------------------------------------------------------------------
# AC8 / AC9 — RESUME. The same plan is re-run with `implement` now passing.
# Resume must land on implement#1 (the first UNRESOLVED occurrence, NOT the
# first `validate`), and the accepted occurrences must not run a second time.
# ---------------------------------------------------------------------------
before_v1="$(ran_count 'validate#1')"
before_lint="$(ran_count 'lint#1')"
out="$(bash "$COORDINATOR" --spec-dir "$SPEC" --cursor "$CURSOR" --format json --max-iterations 3 \
  --phase "validate=$RUN_CMD" \
  --phase "implement=$RUN_CMD" \
  --phase "validate=$RUN_CMD" \
  --independent "lint=$RUN_CMD" 2>&1)"
rc=$?

if [[ "$(jget "$out" 'd["resumedAt"]')" == "implement#1" ]]; then
  pass "AC8: resume starts at the FIRST UNRESOLVED occurrence (implement#1), not at validate#1"
else
  fail "AC8: resume landed on $(jget "$out" 'd["resumedAt"]') instead of implement#1"
fi
if [[ "$(outcome_of 'validate#1')" == "ACCEPTED" && "$(ran_count 'validate#1')" -eq "$before_v1" ]]; then
  pass "AC9: an accepted occurrence is reported ACCEPTED and its command is NOT replayed"
else
  fail "AC9: validate#1 was replayed (outcome $(outcome_of 'validate#1'), runs $(ran_count 'validate#1') vs $before_v1)"
fi
if [[ "$(ran_count 'lint#1')" -eq "$before_lint" ]]; then
  pass "AC9: an accepted INDEPENDENT occurrence is not replayed either"
else
  fail "AC9: lint#1 was replayed"
fi
if [[ "$(outcome_of 'validate#2')" == "RAN_PASS" && "$(ran_count 'validate#2')" -eq 1 ]]; then
  pass "AC8: the SECOND occurrence of a repeated phase runs on its own merits once unblocked"
else
  fail "AC8: validate#2 did not execute after its prerequisite passed"
fi
if [[ "$rc" -eq 0 && "$(jget "$out" 'd["complete"]')" == "True" ]]; then
  pass "AC12: a fully resolved plan exits 0 and reports complete"
else
  fail "AC12: a fully resolved plan did not report completion (exit $rc)"
fi

# ---------------------------------------------------------------------------
# AC12 — PLAN FIDELITY is recorded EVERY iteration, including the failed one.
# ---------------------------------------------------------------------------
iterations="$(python3 -c 'import json,sys;print(len(json.load(open(sys.argv[1]))["iterations"]))' "$CURSOR" 2>/dev/null || printf 'ERR')"
if [[ "$iterations" == "2" ]]; then
  pass "AC12: plan fidelity is recorded for EVERY iteration, including the failed one ($iterations records)"
else
  fail "AC12: expected 2 plan-fidelity records, observed $iterations"
fi
fid_keys="$(python3 -c '
import json,sys
rec = json.load(open(sys.argv[1]))["iterations"][0]
need = ["plannedOccurrences","resolvedOccurrences","outstandingOccurrences","blockedNotRun","failed","notReplayed","offPlanRecords","resumedAt"]
print("ok" if all(k in rec for k in need) else "missing:" + ",".join(k for k in need if k not in rec))
' "$CURSOR" 2>/dev/null || printf 'ERR')"
if [[ "$fid_keys" == "ok" ]]; then
  pass "AC12: each plan-fidelity record names planned, resolved, outstanding, blocked, failed and off-plan work"
else
  fail "AC12: plan-fidelity record is incomplete ($fid_keys)"
fi

# ---------------------------------------------------------------------------
# AC12 — EXHAUSTION NEVER REPORTS SUCCESS.
# ---------------------------------------------------------------------------
EX_CURSOR="$TMP_DIR/exhaust.json"
ex_rc=0
ex_out=""
for _ in 1 2; do
  ex_out="$(bash "$COORDINATOR" --spec-dir "$SPEC" --cursor "$EX_CURSOR" --format json --max-iterations 2 \
    --phase "build=$FAIL_CMD" 2>&1)"
  ex_rc=$?
done
if [[ "$ex_rc" -ne 0 ]]; then
  pass "AC12: an exhausted loop exits NON-ZERO"
else
  fail "AC12: an exhausted loop exited 0"
fi
if [[ "$(jget "$ex_out" 'd["exhausted"]')" == "True" && "$(jget "$ex_out" 'd["complete"]')" == "False" ]]; then
  pass "AC12: exhaustion is reported by name and never as completion"
else
  fail "AC12: exhaustion was not reported honestly (exhausted=$(jget "$ex_out" 'd["exhausted"]'), complete=$(jget "$ex_out" 'd["complete"]'))"
fi

# ---------------------------------------------------------------------------
# AC10 — this is a PRODUCTION consumer: the commands really ran, and the cursor
# it wrote is a real artifact another run reads back.
# ---------------------------------------------------------------------------
if [[ -f "$CURSOR" ]] && [[ "$(python3 -c 'import json,sys;print(len(json.load(open(sys.argv[1]))["occurrences"]))' "$CURSOR" 2>/dev/null)" -ge 4 ]]; then
  pass "AC10: the coordinator persists a real cursor that a later invocation reads back"
else
  fail "AC10: no usable cursor was persisted"
fi
total_runs="$(wc -l < "$RAN" | tr -d ' ')"
if [[ "$total_runs" -ge 4 ]]; then
  pass "AC10: phase resolution drove ACTUAL command execution ($total_runs commands ran)"
else
  fail "AC10: the coordinator resolved phases without executing anything ($total_runs runs)"
fi

# ---------------------------------------------------------------------------
# NO BYPASS.
# ---------------------------------------------------------------------------
for flag in --skip-phase --force --ignore-blocked --replay-accepted; do
  bypass_out="$(bash "$COORDINATOR" --spec-dir "$SPEC" "$flag" --phase "x=true" 2>&1)"
  if [[ $? -eq 2 ]] && printf '%s' "$bypass_out" | grep -qF 'does not exist'; then
    pass "no bypass: \`$flag\` is rejected by name"
  else
    fail "no bypass: \`$flag\` was not rejected"
  fi
done

printf '\n%s: %d passed, %d failed\n' "$NAME" "$passes" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
