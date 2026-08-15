#!/usr/bin/env bash
# bubbles/scripts/evidence-capture-selftest.sh
#
# Hermetic selftest for evidence-capture.sh (IMP-036 SCOPE-6).
#
# The load-bearing property is case 5: --verify must FAIL when the command's
# output changes. If a recorded hash cannot detect drift, the compact form is
# weaker than the transcript it replaces and must not ship.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/evidence-capture.sh"
NAME="evidence-capture-selftest"

failures=0
checks=0
ok() { checks=$((checks + 1)); printf '  ok   %s\n' "$1"; }
bad() {
  checks=$((checks + 1)); failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"; [[ $# -gt 1 ]] && printf '       %s\n' "$2"
}

# --- 1. records command, exit code and hash ----------------------------------
out="$(bash "$TARGET" --label "demo" -- printf 'a\nb\nc\n' 2>&1)"
if printf '%s' "$out" | grep -q '^exit: 0$' &&
  printf '%s' "$out" | grep -q '^lines: 3$' &&
  printf '%s' "$out" | grep -qE '^sha256: [0-9a-f]{64}$'; then
  ok "records command, exit code, line count and a sha256"
else
  bad "records the basics" "$(printf '%s' "$out" | tr '\n' '|')"
fi

# --- 2. short output is shown in full ----------------------------------------
if printf '%s' "$out" | grep -q -- '--- output ---' &&
  printf '%s' "$out" | grep -qx 'b'; then
  ok "short output is emitted in full, not truncated"
else
  bad "short output shown in full" "$(printf '%s' "$out" | tr '\n' '|')"
fi

# --- 3. long output is head/tail trimmed with an explicit omission note -------
long="$(bash "$TARGET" --lines 2 -- seq 1 50 2>&1)"
if printf '%s' "$long" | grep -q -- '--- first 2 ---' &&
  printf '%s' "$long" | grep -q 'omitted 46 line(s)' &&
  printf '%s' "$long" | grep -q -- '--- last 2 ---'; then
  ok "long output is trimmed and states how many lines were omitted"
else
  bad "long output trimmed" "$(printf '%s' "$long" | tr '\n' '|')"
fi

# --- 4. a FAILING command still produces evidence and propagates its code -----
set +e
fail_out="$(bash "$TARGET" -- sh -c 'echo boom; exit 7' 2>&1)"
rc=$?
set -e
if [[ "$rc" -eq 7 ]] && printf '%s' "$fail_out" | grep -q '^exit: 7$' &&
  printf '%s' "$fail_out" | grep -qx 'boom'; then
  ok "failing command still emits evidence and propagates exit 7"
else
  bad "failing command evidence" "rc=$rc $(printf '%s' "$fail_out" | tr '\n' '|')"
fi

# --- 5. ADVERSARIAL: --verify must DETECT changed output ---------------------
# Without this, the hash is decoration and the compact form would be weaker than
# a transcript.
digest="$(bash "$TARGET" -- printf 'stable\n' 2>&1 | awk '/^sha256: /{print $2}')"
set +e
bash "$TARGET" --verify "$digest" -- printf 'stable\n' >/dev/null 2>&1
same_rc=$?
bash "$TARGET" --verify "$digest" -- printf 'CHANGED\n' >/dev/null 2>&1
diff_rc=$?
set -e
if [[ "$same_rc" -eq 0 && "$diff_rc" -eq 3 ]]; then
  ok "--verify passes on identical output and FAILS (3) when it changes"
else
  bad "--verify detects drift" "same=$same_rc changed=$diff_rc (want 0 and 3)"
fi

# --- 6. stderr is captured, not dropped --------------------------------------
err_out="$(bash "$TARGET" -- sh -c 'echo to-stderr >&2' 2>&1)"
if printf '%s' "$err_out" | grep -qx 'to-stderr'; then
  ok "stderr is interleaved into the evidence, not discarded"
else
  bad "stderr captured" "$(printf '%s' "$err_out" | tr '\n' '|')"
fi

# --- 7. bypass-shaped flags are refused --------------------------------------
set +e
bash "$TARGET" --fake -- true >/dev/null 2>&1
rc=$?
set -e
if [[ "$rc" -eq 2 ]]; then
  ok "bypass-shaped flag refused with exit 2"
else
  bad "bypass flag refused" "exit was $rc"
fi

# --- 8. a missing command is a usage error -----------------------------------
set +e
bash "$TARGET" --label x >/dev/null 2>&1
rc=$?
set -e
if [[ "$rc" -eq 2 ]]; then
  ok "no command after -- is a usage error"
else
  bad "missing command is usage error" "exit was $rc"
fi

# --- 9. emits a re-runnable verify hint --------------------------------------
if printf '%s' "$out" | grep -q '<!-- verify: bash bubbles/scripts/evidence-capture.sh --verify'; then
  ok "block carries a re-runnable verify command"
else
  bad "verify hint emitted" "$(printf '%s' "$out" | tr '\n' '|')"
fi

# --- 10. ADVERSARIAL: a failure line inside the omitted region survives -------
# The whole case for preferring the bounded block over a transcript collapses if
# trimming can swallow the line that explains the exit code. Line 4 of 7 falls
# strictly inside the omitted middle at --lines 2.
mid_fail="$(bash "$TARGET" --lines 2 -- sh -c 'echo a; echo b; echo c; echo "FAIL: buried signal"; echo d; echo e; echo f' 2>&1)"
if printf '%s' "$mid_fail" | grep -q -- '--- failure-shaped lines from the omitted region ---' &&
  printf '%s' "$mid_fail" | grep -q 'FAIL: buried signal'; then
  ok "a failure line in the omitted region is lifted out, not swallowed"
else
  bad "omitted-region failure line surfaced" "$(printf '%s' "$mid_fail" | tr '\n' '|')"
fi

# --- 11. clean long output gains no failure section --------------------------
# Guards case 10 against the opposite defect: a section that always appears
# proves nothing about detection.
clean_long="$(bash "$TARGET" --lines 2 -- seq 1 20 2>&1)"
if ! printf '%s' "$clean_long" | grep -q -- 'failure-shaped lines'; then
  ok "clean output emits no failure section"
else
  bad "clean output emits no failure section" "$(printf '%s' "$clean_long" | tr '\n' '|')"
fi

# --- 12. the diagnostic escalation is explicit, stamped, and still bounded ----
# SCOPE-7's decision was one default plus a per-invocation escalation, NOT a
# second verbosity mode. These cases hold that line: opting in must be visible
# in the block, and it must not become an unbounded transcript paste.
diag="$(bash "$TARGET" --diagnostic -- seq 1 6 2>&1)"
if printf '%s' "$diag" | grep -q '^escalation: diagnostic' &&
  printf '%s' "$diag" | grep -qx '4'; then
  ok "--diagnostic emits the full output and stamps the escalation"
else
  bad "--diagnostic stamps and emits" "$(printf '%s' "$diag" | tr '\n' '|')"
fi

if ! printf '%s' "$out" | grep -q '^escalation:'; then
  ok "a normal capture carries no escalation stamp"
else
  bad "normal capture is unstamped" "$(printf '%s' "$out" | tr '\n' '|')"
fi

# --- 13. ADVERSARIAL: the escalation still has a ceiling ---------------------
# "Unbounded on request" is exactly how a bounded default erodes back into the
# paste it replaced.
big="$(bash "$TARGET" --diagnostic -- seq 1 2500 2>&1)"
big_lines="$(printf '%s' "$big" | grep -c '')"
if printf '%s' "$big" | grep -q 'diagnostic ceiling' &&
  printf '%s' "$big" | grep -q 'omitted 500 line(s) beyond the diagnostic ceiling' &&
  [[ "$big_lines" -lt 2500 ]]; then
  ok "--diagnostic remains bounded by a stated ceiling"
else
  bad "--diagnostic bounded by ceiling" "emitted $big_lines line(s)"
fi

# --- 14. ADVERSARIAL: TERM preserves partial evidence and records interruption -
# A timeout signals the wrapper while its child is running. The signal handler
# must not delete the capture before line count, hash, and bounded output are
# emitted; that produced a misleading empty block plus missing-file errors.
set +e
term_out="$(bash "$TARGET" -- sh -c 'printf "before-signal\n"; kill -TERM "$PPID"; printf "after-signal\n"' 2>&1)"
term_rc=$?
set -e
if [[ "$term_rc" -eq 143 ]] &&
  printf '%s' "$term_out" | grep -q '^exit: 143$' &&
  printf '%s' "$term_out" | grep -qE '^sha256: [0-9a-f]{64}$' &&
  printf '%s' "$term_out" | grep -qx 'before-signal' &&
  ! printf '%s' "$term_out" | grep -q 'No such file or directory'; then
  ok "TERM preserves captured output and emits an interrupted evidence block"
else
  bad "TERM preserves interrupted evidence" "rc=$term_rc $(printf '%s' "$term_out" | tr '\n' '|')"
fi

printf '\n%s: %d/%d checks passed\n' "$NAME" "$((checks - failures))" "$checks"
if [[ "$failures" -gt 0 ]]; then
  printf '%s: FAILED\n' "$NAME"
  exit 1
fi
printf '%s: OK\n' "$NAME"
exit 0
