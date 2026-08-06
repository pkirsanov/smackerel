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
out="$(bash "$TARGET" report --repo-root "$r1" 2>&1)"
if printf '%s' "$out" | grep -qE '^\s+G024\s+2\s+1\s+1' &&
  printf '%s' "$out" | grep -q 'NEVER rejected anything'; then
  ok "report aggregates per gate and names never-rejecting gates"
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
out="$(bash "$TARGET" report --repo-root "$r1" --json 2>&1)"
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
out="$(bash "$TARGET" report --repo-root "$r14" 2>&1)"
if printf '%s' "$out" | grep -q 'runs using parent-expansion' &&
  printf '%s' "$out" | grep -q 'phases parent-expanded in total: 3'; then
  ok "report surfaces parent-expansion rate"
else
  bad "report surfaces expansion rate" "$(printf '%s' "$out" | tr '\n' '|')"
fi

printf '\n%s: %d/%d checks passed\n' "$NAME" "$((checks - failures))" "$checks"
if [[ "$failures" -gt 0 ]]; then
  printf '%s: FAILED\n' "$NAME"
  exit 1
fi
printf '%s: OK\n' "$NAME"
exit 0
