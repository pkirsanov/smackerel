#!/usr/bin/env bash
# bubbles/scripts/gate-hit-log-selftest.sh
#
# Hermetic selftest for gate-hit-log.sh (IMP-036 SCOPE-4).
#
# The load-bearing property is NOT "it writes a line". It is that telemetry can
# never break a guard. Case 4 is the one that matters: an unwritable log target
# must still exit 0, because the alternative is a framework that starts refusing
# commits when a directory is read-only.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/gate-hit-log.sh"
NAME="gate-hit-log-selftest"

failures=0
checks=0

ok() { checks=$((checks + 1)); printf '  ok   %s\n' "$1"; }
bad() {
  checks=$((checks + 1))
  failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"
  [[ $# -gt 1 ]] && printf '       %s\n' "$2"
}

WORK="$(mktemp -d 2>/dev/null)" || { printf '%s: cannot create temp dir\n' "$NAME" >&2; exit 1; }
# shellcheck disable=SC2317  # invoked indirectly by the EXIT/INT/TERM trap
cleanup() { [[ -n "${WORK:-}" && -d "$WORK" ]] && rm -rf "$WORK"; }
trap cleanup EXIT INT TERM

log_of() { printf '%s/.specify/runtime/gate-hits.jsonl' "$1"; }

# --- 1. append writes one line per gate id -----------------------------------
r1="$WORK/r1"
mkdir -p "$r1"
bash "$TARGET" append --repo-root "$r1" --spec "specs/001-x" --mode full-delivery \
  --target-status "done" --verdict PASS --exit-status 0 \
  --passed "G024 G025" --failed "G028" >/dev/null 2>&1
lines=$(wc -l <"$(log_of "$r1")" 2>/dev/null | tr -d ' ')
if [[ "$lines" == "3" ]]; then
  ok "append writes one line per gate id (3 gates -> 3 lines)"
else
  bad "append writes one line per gate id" "expected 3 lines, got '${lines:-none}'"
fi

# --- 2. outcome is recorded per gate, not per run ----------------------------
if grep -q '"gate":"G028","outcome":"fail"' "$(log_of "$r1")" 2>/dev/null &&
  grep -q '"gate":"G024","outcome":"pass"' "$(log_of "$r1")" 2>/dev/null; then
  ok "each gate carries its own pass/fail outcome"
else
  bad "each gate carries its own pass/fail outcome"
fi

# --- 3. append-only: a second run adds, never truncates ----------------------
bash "$TARGET" append --repo-root "$r1" --spec "specs/002-y" --mode bugfix-fastlane \
  --target-status "done" --verdict FAIL --exit-status 1 --failed "G024" >/dev/null 2>&1
lines=$(wc -l <"$(log_of "$r1")" 2>/dev/null | tr -d ' ')
if [[ "$lines" == "4" ]]; then
  ok "log is append-only (3 + 1 = 4 lines)"
else
  bad "log is append-only" "expected 4 lines, got '${lines:-none}'"
fi

# --- 4. ADVERSARIAL: unwritable target must NOT fail the caller --------------
blocker="$WORK/not-a-dir"
printf 'x' >"$blocker"
set +e
bash "$TARGET" append --repo-root "$blocker/nested" --spec s --mode m \
  --target-status "done" --verdict PASS --exit-status 0 --passed "G001" >/dev/null 2>&1
rc=$?
set -e
if [[ "$rc" -eq 0 ]]; then
  ok "unwritable log target still exits 0 (telemetry never breaks a guard)"
else
  bad "unwritable log target still exits 0" "exit was $rc"
fi

# --- 5. opt-out writes nothing ------------------------------------------------
r5="$WORK/r5"
mkdir -p "$r5"
BUBBLES_GATE_HIT_LOG=off bash "$TARGET" append --repo-root "$r5" --spec s --mode m \
  --target-status "done" --verdict PASS --exit-status 0 --passed "G001" >/dev/null 2>&1
if [[ ! -f "$(log_of "$r5")" ]]; then
  ok "BUBBLES_GATE_HIT_LOG=off writes nothing"
else
  bad "BUBBLES_GATE_HIT_LOG=off writes nothing" "log file was created"
fi

# --- 6. ADVERSARIAL: malformed gate ids are not recorded ---------------------
r6="$WORK/r6"
mkdir -p "$r6"
bash "$TARGET" append --repo-root "$r6" --spec s --mode m --target-status "done" \
  --verdict PASS --exit-status 0 --passed "notagate G12 G1234 gG001 G007" >/dev/null 2>&1
lines=$(wc -l <"$(log_of "$r6")" 2>/dev/null | tr -d ' ')
if [[ "$lines" == "1" ]] && grep -q '"gate":"G007"' "$(log_of "$r6")" 2>/dev/null; then
  ok "malformed gate ids rejected, only G007 recorded"
else
  bad "malformed gate ids rejected" "expected 1 line (G007), got '${lines:-none}'"
fi

# --- 7. report aggregates hits, passes and fails -----------------------------
# These fixtures live under a temp root, which IMP-042 SCOPE-17 classes as
# `fixture`. This case is about aggregation arithmetic, so it reads every class;
# the filtering behaviour itself is cases 17-22.
out="$(bash "$TARGET" report --repo-root "$r1" --all-classes 2>&1)"
# G024 appears twice: permitted on the first run, then refused on a run that did
# not proceed. Two records, two firings, one prevention.
if printf '%s' "$out" | grep -qE '^\s+G024\s+2\s+2\s+0\s+1' &&
  printf '%s' "$out" | grep -q 'gates that FIRED but never prevented'; then
  ok "report aggregates per gate and names gates that fired without preventing"
else
  bad "report aggregates per gate" "$(printf '%s' "$out" | tr '\n' '|')"
fi

# --- 8. report is graceful with no log ---------------------------------------
r8="$WORK/r8"
mkdir -p "$r8"
set +e
out="$(bash "$TARGET" report --repo-root "$r8" 2>&1)"
rc=$?
set -e
if [[ "$rc" -eq 0 ]] && printf '%s' "$out" | grep -q 'no telemetry yet'; then
  ok "report with no log exits 0 with an honest message"
else
  bad "report with no log exits 0" "exit $rc: $out"
fi

# --- 9. json report shape -----------------------------------------------------
out="$(bash "$TARGET" report --repo-root "$r1" --json --all-classes 2>&1)"
if printf '%s' "$out" | grep -q '"schemaVersion":"gate-hit/v1"' &&
  printf '%s' "$out" | grep -q '"logPresent":true'; then
  ok "json report carries schemaVersion and logPresent"
else
  bad "json report shape" "$out"
fi

# --- 10. no args is a usage error --------------------------------------------
set +e
bash "$TARGET" >/dev/null 2>&1
rc=$?
set -e
if [[ "$rc" -eq 2 ]]; then
  ok "no subcommand exits 2 (usage error)"
else
  bad "no subcommand exits 2" "exit was $rc"
fi

# --- 11. ADVERSARIAL: absolute spec paths are relativised (PII hygiene) ------
r11="$WORK/r11"
mkdir -p "$r11"
bash "$TARGET" append --repo-root "$r11" --spec "$r11/specs/001-x" --mode m \
  --target-status "done" --verdict PASS --exit-status 0 --passed "G001" >/dev/null 2>&1
if grep -q '"spec":"specs/001-x"' "$(log_of "$r11")" 2>/dev/null &&
  ! grep -q "\"spec\":\"$r11" "$(log_of "$r11")" 2>/dev/null; then
  ok "absolute spec path stored repo-relative, no home directory leaked"
else
  bad "absolute spec path relativised" "$(tr '\n' '|' <"$(log_of "$r11")" 2>/dev/null)"
fi

# --- 12. sourced path works and does not clobber the caller's namespace ------
# This is the PRODUCTION path: state-transition-guard.sh sources this file so a
# guard run costs no subprocess. If sourcing ever redefined the caller's
# usage()/main()/SCRIPT_DIR the guard would break in ways a CLI-only test misses.
r12="$WORK/r12"
mkdir -p "$r12"
sourced_out="$(
  usage() { printf 'CALLER_USAGE'; }
  main() { printf 'CALLER_MAIN'; }
  SCRIPT_DIR="/caller/dir"
  # shellcheck disable=SC1090
  source "$TARGET"
  bubbles_gate_hit_append --repo-root "$r12" --spec "specs/003-z" --mode m \
    --target-status "done" --verdict PASS --exit-status 0 --passed "G042"
  printf '%s|%s|%s' "$(usage)" "$(main)" "$SCRIPT_DIR"
)"
if [[ "$sourced_out" == "CALLER_USAGE|CALLER_MAIN|/caller/dir" ]] &&
  grep -q '"gate":"G042"' "$(log_of "$r12")" 2>/dev/null; then
  ok "sourced path appends without clobbering caller usage/main/SCRIPT_DIR"
else
  bad "sourced path preserves caller namespace" "got '$sourced_out'"
fi

# --- 13. sourcing does not run the CLI as a side effect ----------------------
side_effect="$(
  # shellcheck disable=SC1090
  source "$TARGET" 2>&1
)"
if [[ -z "$side_effect" ]]; then
  ok "sourcing produces no output (CLI entrypoint is not auto-invoked)"
else
  bad "sourcing produces no output" "got '$side_effect'"
fi

# --- 14. parent-expansion run record (IMP-036 SCOPE-2) -----------------------
r14="$WORK/r14"
mkdir -p "$r14"
bash "$TARGET" append --repo-root "$r14" --spec "specs/004-w" --mode full-delivery \
  --target-status "done" --verdict PASS --exit-status 0 --passed "G022" \
  --parent-expanded 3 >/dev/null 2>&1
if grep -q '"kind":"run"' "$(log_of "$r14")" 2>/dev/null &&
  grep -q '"parentExpanded":3' "$(log_of "$r14")" 2>/dev/null &&
  grep -q '"kind":"gate"' "$(log_of "$r14")" 2>/dev/null; then
  ok "parent-expansion emits a run record alongside gate records"
else
  bad "parent-expansion run record" "$(tr '\n' '|' <"$(log_of "$r14")" 2>/dev/null)"
fi

# --- 15. ADVERSARIAL: a non-numeric expansion count writes no run record -----
r15="$WORK/r15"
mkdir -p "$r15"
bash "$TARGET" append --repo-root "$r15" --spec s --mode m --target-status "done" \
  --verdict PASS --exit-status 0 --passed "G001" --parent-expanded "lots" >/dev/null 2>&1
if ! grep -q '"kind":"run"' "$(log_of "$r15")" 2>/dev/null &&
  grep -q '"kind":"gate"' "$(log_of "$r15")" 2>/dev/null; then
  ok "non-numeric expansion count is ignored, gate records still written"
else
  bad "non-numeric expansion count ignored" "$(tr '\n' '|' <"$(log_of "$r15")" 2>/dev/null)"
fi

# --- 16. report surfaces the expansion rate ----------------------------------
out="$(bash "$TARGET" report --repo-root "$r14" --all-classes 2>&1)"
if printf '%s' "$out" | grep -q 'runs using parent-expansion' &&
  printf '%s' "$out" | grep -q 'phases parent-expanded in total: 3'; then
  ok "report surfaces parent-expansion rate"
else
  bad "report surfaces expansion rate" "$(printf '%s' "$out" | tr '\n' '|')"
fi

# =============================================================================
# IMP-042 SCOPE-17 - source classing.
#
# The gate-hit log is the ONLY evidence base for retiring a gate. Before source
# classing, a selftest driving the guard through a fixture repo wrote records
# indistinguishable from production ones, so "G0xx rejected something 40 times"
# could be describing this very file. Case 18 is the one that matters: the
# report must DROP fixture records, not merely label them.
# =============================================================================

# --- 17. a temp-dir root is classed as a fixture WITHOUT being asked ----------
# A fixture that forgets to declare itself is exactly the record that pollutes
# the report, so the class is derived rather than requested.
if grep -q '"sourceClass":"fixture"' "$(log_of "$r1")" 2>/dev/null &&
  ! grep -q '"sourceClass":"product"' "$(log_of "$r1")" 2>/dev/null; then
  ok "a temp-dir repo root is auto-classed fixture with no declaration"
else
  bad "a temp-dir repo root is auto-classed fixture" \
    "$(grep -o '"sourceClass":"[^"]*"' "$(log_of "$r1")" 2>/dev/null | sort -u | tr '\n' ' ')"
fi

# --- 18. ADVERSARIAL: a fixture-only log must report ZERO gates ---------------
# If this ever passes fixture data through, every retirement decision built on
# this log is fiction.
out="$(bash "$TARGET" report --repo-root "$r1" --json 2>&1)"
if printf '%s' "$out" | grep -q '"gates":\[\]'; then
  ok "a fixture-only log reports zero gates under the default product filter"
else
  bad "fixture records are excluded by default" "$out"
fi

# --- 19. the exclusion is stated, so a filtered view is never silent ----------
if printf '%s' "$out" | grep -q '"excludedRecords":[1-9]' &&
  printf '%s' "$out" | grep -q '"sourceClass":"product"'; then
  ok "the report names its filter and counts what it excluded"
else
  bad "report states filter and exclusion count" "$out"
fi

# --- 20. run records carry the class too -------------------------------------
# Without this the parent-expansion rate would stay contaminated while the gate
# counts looked clean.
if grep '"kind":"run"' "$(log_of "$r14")" 2>/dev/null | grep -q '"sourceClass":'; then
  ok "run records carry the source class, not just gate records"
else
  bad "run records carry the source class"
fi

# --- 21. a typo'd class is demoted, never trusted as product -----------------
# Failing open here would let one misspelling promote test records into
# retirement evidence.
r21="$WORK/r21"
mkdir -p "$r21"
BUBBLES_GATE_HIT_SOURCE_CLASS=prodcut bash "$TARGET" append --repo-root "$r21" \
  --spec "specs/001-x" --mode full-delivery --target-status "done" \
  --verdict PASS --exit-status 0 --passed "G011" >/dev/null 2>&1
if grep -q '"sourceClass":"fixture"' "$(log_of "$r21")" 2>/dev/null &&
  ! grep -q 'prodcut' "$(log_of "$r21")" 2>/dev/null; then
  ok "an unrecognised declared class is demoted to fixture, not trusted"
else
  bad "unrecognised class is demoted to fixture" \
    "$(grep -o '"sourceClass":"[^"]*"' "$(log_of "$r21")" 2>/dev/null | tr '\n' ' ')"
fi

# --- 22. pre-classing history still counts as product ------------------------
# Dropping unlabelled records would silently discard every observation gathered
# before this change shipped.
r22="$WORK/r22"
mkdir -p "$r22/.specify/runtime"
printf '%s\n' '{"schemaVersion":"gate-hit/v1","kind":"gate","ts":"2026-01-01T00:00:00Z","gate":"G099","outcome":"fail","spec":"specs/001-x","mode":"full-delivery","targetStatus":"done","guardVerdict":"FAIL","exitStatus":"1"}' \
  >"$(log_of "$r22")"
out="$(bash "$TARGET" report --repo-root "$r22" --json 2>&1)"
if printf '%s' "$out" | grep -q '"gate":"G099"'; then
  ok "records written before source classing still count as product"
else
  bad "legacy unlabelled records still count as product" "$out"
fi

# --- IMP-047 S-A: firing and prevention are separate facts -------------------
#
# Every case below fails if the fired/prevented split is reverted, because the
# old contract wrote neither field and folded "credited without evaluation" into
# `pass`.

# --- 23. a fired-and-permitted gate is distinguishable from one that never fired
r23="$WORK/r23"
mkdir -p "$r23"
bash "$TARGET" append --repo-root "$r23" --spec "specs/001-x" --mode full-delivery \
  --target-status "done" --verdict PASS --exit-status 0 \
  --passed "G024" --not-evaluated "G025" >/dev/null 2>&1
if grep -q '"gate":"G024","outcome":"pass","fired":true,"prevented":false' "$(log_of "$r23")" 2>/dev/null &&
  grep -q '"gate":"G025","outcome":"not-evaluated","fired":false,"prevented":false' "$(log_of "$r23")" 2>/dev/null; then
  ok "a gate that fired and permitted is distinguishable from one that never fired"
else
  bad "fired and never-fired are distinguishable" \
    "$(cat "$(log_of "$r23")" 2>/dev/null)"
fi

# --- 24. ADVERSARIAL: a refusal on a run that still proceeded is NOT prevention
# Recording this as a prevention would manufacture retirement evidence out of a
# gate that changed no outcome.
r24="$WORK/r24"
mkdir -p "$r24"
bash "$TARGET" append --repo-root "$r24" --spec "specs/001-x" --mode full-delivery \
  --target-status "done" --verdict PASS --exit-status 0 --failed "G028" >/dev/null 2>&1
if grep -q '"gate":"G028","outcome":"fail","fired":true,"prevented":false' "$(log_of "$r24")" 2>/dev/null; then
  ok "a refusal on a run that still proceeded is fired but NOT prevented"
else
  bad "non-blocking refusal is not recorded as prevention" \
    "$(cat "$(log_of "$r24")" 2>/dev/null)"
fi

# --- 25. a refusal that stopped the run IS prevention ------------------------
r25="$WORK/r25"
mkdir -p "$r25"
bash "$TARGET" append --repo-root "$r25" --spec "specs/001-x" --mode full-delivery \
  --target-status "done" --verdict BLOCKED --exit-status 2 --failed "G028" >/dev/null 2>&1
if grep -q '"gate":"G028","outcome":"fail","fired":true,"prevented":true' "$(log_of "$r25")" 2>/dev/null; then
  ok "a refusal that stopped the run is recorded as prevention"
else
  bad "blocking refusal is recorded as prevention" \
    "$(cat "$(log_of "$r25")" 2>/dev/null)"
fi

# --- 26. the report separates prevention from firing -------------------------
r26="$WORK/r26"
mkdir -p "$r26"
bash "$TARGET" append --repo-root "$r26" --spec "specs/001-x" --mode full-delivery \
  --target-status "done" --verdict BLOCKED --exit-status 2 \
  --passed "G024" --failed "G028" --not-evaluated "G025" >/dev/null 2>&1
out="$(bash "$TARGET" report --repo-root "$r26" --json --all-classes 2>&1)"
if printf '%s' "$out" | grep -q '"gate":"G028","hits":1,"passes":0,"fails":1,"fired":1,"notFired":0,"prevented":1' &&
  printf '%s' "$out" | grep -q '"gate":"G024","hits":1,"passes":1,"fails":0,"fired":1,"notFired":0,"prevented":0' &&
  printf '%s' "$out" | grep -q '"gate":"G025","hits":1,"passes":0,"fails":0,"fired":0,"notFired":1,"prevented":0'; then
  ok "the report counts fired, notFired and prevented as separate per-gate facts"
else
  bad "report separates fired, notFired and prevented" "$out"
fi

# --- 27. the retirement basis is stated as prevention, not as a pass count ----
out="$(bash "$TARGET" report --repo-root "$r26" --all-classes 2>&1)"
if printf '%s' "$out" | grep -q 'gates that PREVENTED at least once: 1' &&
  printf '%s' "$out" | grep -q 'gates that FIRED but never prevented: 1' &&
  printf '%s' "$out" | grep -q 'NEVER FIRED): 1' &&
  printf '%s' "$out" | grep -q 'Prevention is the only valid basis for retirement'; then
  ok "the text report names prevention as the retirement basis"
else
  bad "text report names prevention as the retirement basis" "$out"
fi

# --- 28. ADVERSARIAL: legacy records are derived, counted, and never relabelled
# A record predating the split carries neither field. Dropping it would discard
# history; silently treating it as directly observed would overstate the
# evidence. It must be derived AND declared as derived.
r28="$WORK/r28"
mkdir -p "$r28/.specify/runtime"
{
  printf '%s\n' '{"schemaVersion":"gate-hit/v1","kind":"gate","ts":"2026-01-01T00:00:00Z","sourceClass":"product","gate":"G099","outcome":"fail","spec":"specs/001-x","mode":"full-delivery","targetStatus":"done","guardVerdict":"FAIL","exitStatus":"1"}'
  printf '%s\n' '{"schemaVersion":"gate-hit/v1","kind":"gate","ts":"2026-01-01T00:00:00Z","sourceClass":"product","gate":"G098","outcome":"fail","spec":"specs/001-x","mode":"full-delivery","targetStatus":"done","guardVerdict":"PASS","exitStatus":"0"}'
} >"$(log_of "$r28")"
out="$(bash "$TARGET" report --repo-root "$r28" --json 2>&1)"
if printf '%s' "$out" | grep -q '"legacyDerivedRecords":2' &&
  printf '%s' "$out" | grep -q '"gate":"G099","hits":1,"passes":0,"fails":1,"fired":1,"notFired":0,"prevented":1,"legacyDerived":1' &&
  printf '%s' "$out" | grep -q '"gate":"G098","hits":1,"passes":0,"fails":1,"fired":1,"notFired":0,"prevented":0,"legacyDerived":1'; then
  ok "legacy records derive fired/prevented from their own fields and are counted as derived"
else
  bad "legacy records are derived and declared" "$out"
fi

# --- 29. the text report discloses how much evidence is legacy-derived -------
out="$(bash "$TARGET" report --repo-root "$r28" 2>&1)"
if printf '%s' "$out" | grep -q '2 record(s) predate the fired/prevented fields'; then
  ok "the report discloses legacy-derived evidence instead of presenting it as observed"
else
  bad "report discloses legacy-derived evidence" "$out"
fi

printf '\n%s: %d/%d checks passed\n' "$NAME" "$((checks - failures))" "$checks"
if [[ "$failures" -gt 0 ]]; then
  printf '%s: FAILED\n' "$NAME"
  exit 1
fi
printf '%s: OK\n' "$NAME"
exit 0
