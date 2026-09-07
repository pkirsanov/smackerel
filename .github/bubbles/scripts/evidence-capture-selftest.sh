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
EMPTY_SHA256="e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
ok() { checks=$((checks + 1)); printf '  ok   %s\n' "$1"; }
bad() {
  checks=$((checks + 1)); failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"; [[ $# -gt 1 ]] && printf '       %s\n' "$2"
}

# --- 0. zero-output metadata is canonical and diagnostic-free ---------------
# BUG-037: grep -c emitted a valid zero and then its fallback emitted a second
# zero. Command substitution retained both lines, so arithmetic consumers wrote
# diagnostics into an otherwise valid evidence block.
set +e
empty_success="$(bash "$TARGET" -- true 2>&1)"
empty_success_rc=$?
empty_failure="$(bash "$TARGET" -- sh -c 'exit 7' 2>&1)"
empty_failure_rc=$?
set -e

if [[ "$empty_success_rc" -eq 0 ]] &&
  printf '%s' "$empty_success" | grep -q '^exit: 0$' &&
  printf '%s' "$empty_success" | grep -q '^lines: 0$' &&
  printf '%s' "$empty_success" | grep -q "^sha256: $EMPTY_SHA256$" &&
  printf '%s' "$empty_success" | grep -q -- '^--- output ---$' &&
  ! printf '%s' "$empty_success" | grep -q 'arithmetic' &&
  ! printf '%s' "$empty_success" | grep -qx '0'; then
  ok "successful zero-output capture emits canonical metadata without diagnostics"
else
  bad "successful zero-output metadata" "rc=$empty_success_rc $(printf '%s' "$empty_success" | tr '\n' '|')"
fi

if [[ "$empty_failure_rc" -eq 7 ]] &&
  printf '%s' "$empty_failure" | grep -q '^exit: 7$' &&
  printf '%s' "$empty_failure" | grep -q '^lines: 0$' &&
  printf '%s' "$empty_failure" | grep -q "^sha256: $EMPTY_SHA256$" &&
  printf '%s' "$empty_failure" | grep -q -- '^--- output ---$' &&
  ! printf '%s' "$empty_failure" | grep -q 'arithmetic' &&
  ! printf '%s' "$empty_failure" | grep -qx '0'; then
  ok "failing zero-output capture emits canonical metadata and propagates exit 7"
else
  bad "failing zero-output metadata" "rc=$empty_failure_rc $(printf '%s' "$empty_failure" | tr '\n' '|')"
fi

# --- 0b. empty-output verification retains match and mismatch contracts -----
set +e
empty_match="$(bash "$TARGET" --verify "$EMPTY_SHA256" -- true 2>&1)"
empty_match_rc=$?
empty_mismatch="$(bash "$TARGET" --verify "$(printf '0%.0s' {1..64})" -- true 2>&1)"
empty_mismatch_rc=$?
set -e
if [[ "$empty_match_rc" -eq 0 ]] &&
  [[ "$empty_match" == "[evidence-capture] VERIFIED - output still hashes to $EMPTY_SHA256" ]] &&
  [[ "$empty_mismatch_rc" -eq 3 ]] &&
  printf '%s' "$empty_mismatch" | grep -q '^\[evidence-capture\] MISMATCH$' &&
  printf '%s' "$empty_mismatch" | grep -q "^  observed: $EMPTY_SHA256$" &&
  ! printf '%s' "$empty_match$empty_mismatch" | grep -q 'arithmetic'; then
  ok "empty-output --verify match and mismatch preserve exits 0 and 3"
else
  bad "empty-output verify contracts" "match=$empty_match_rc mismatch=$empty_mismatch_rc match_out=$(printf '%s' "$empty_match" | tr '\n' '|') mismatch_out=$(printf '%s' "$empty_mismatch" | tr '\n' '|')"
fi

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

# --- 14. ADVERSARIAL: TERM stops the child tree and preserves partial evidence -
# A timeout signals the wrapper while its child is running. The child below
# cannot exit unless the wrapper forwards TERM; the outer deadline prevents a
# broken implementation from hanging this selftest forever.
#
# The deadline is a BACKSTOP and must not rewrite the command's exit code: the
# assertions below require the 143 the child's own `kill -TERM "$PPID"`
# produces. guard-lib's bubbles_run_with_timeout is deliberately NOT used here
# because it normalizes 143 to 124 to match GNU timeout, which would mask
# exactly the signal this case exists to observe. Bare `timeout` is not an
# option either -- stock macOS ships none, and it exited 127 here.
run_with_deadline() {
  local secs="$1"
  shift
  if command -v timeout >/dev/null 2>&1; then
    timeout --kill-after=1 "$secs" "$@"
  elif command -v gtimeout >/dev/null 2>&1; then
    gtimeout --kill-after=1 "$secs" "$@"
  else
    # alarm(2) survives exec, so the deadline still applies while the exec'd
    # command keeps its own exit status.
    /usr/bin/perl -e 'alarm shift @ARGV; exec @ARGV' "$secs" "$@"
  fi
}
set +e
# PPID expands inside the child shell.
# shellcheck disable=SC2016
term_out="$(run_with_deadline 5 bash "$TARGET" -- bash -c 'trap '\''printf "child-terminated\n"; exit 0'\'' TERM; printf "before-signal\n"; kill -TERM "$PPID"; while :; do :; done' 2>&1)"
term_rc=$?
set -e
if [[ "$term_rc" -eq 143 ]] &&
  printf '%s' "$term_out" | grep -q '^exit: 143$' &&
  printf '%s' "$term_out" | grep -qE '^sha256: [0-9a-f]{64}$' &&
  printf '%s' "$term_out" | grep -qx 'before-signal' &&
  printf '%s' "$term_out" | grep -qx 'child-terminated' &&
  ! printf '%s' "$term_out" | grep -q 'No such file or directory'; then
  ok "TERM stops the child process group and emits preserved interrupted evidence"
else
  bad "TERM stops children and preserves interrupted evidence" "rc=$term_rc $(printf '%s' "$term_out" | tr '\n' '|')"
fi

# --- 15. ADVERSARIAL: a completed parent cannot leave a descendant behind ---
# Nested wrappers can exit before a background lock holder. Evidence capture
# owns the complete command tree, so returning to the caller must also mean the
# process group has drained.
descendant_pid_file="$(mktemp)"
set +e
# Positional parameters expand inside the child shell.
# shellcheck disable=SC2016
descendant_out="$(run_with_deadline 5 bash "$TARGET" -- bash -c 'bash -c '\''trap "exit 0" TERM; while :; do :; done'\'' & printf "%s\n" "$!" >"$1"' _ "$descendant_pid_file" 2>&1)"
descendant_rc=$?
set -e
descendant_pid="$(cat "$descendant_pid_file")"
rm -f "$descendant_pid_file"
if [[ "$descendant_rc" -eq 0 ]] &&
  printf '%s' "$descendant_out" | grep -q '^exit: 0$' &&
  [[ "$descendant_pid" =~ ^[0-9]+$ ]] &&
  ! kill -0 "$descendant_pid" 2>/dev/null; then
  ok "completed commands leave no background descendant behind"
else
  if [[ "$descendant_pid" =~ ^[0-9]+$ ]]; then
    kill -KILL "$descendant_pid" 2>/dev/null || true
  fi
  bad "completed command tree cleanup" "rc=$descendant_rc pid=$descendant_pid $(printf '%s' "$descendant_out" | tr '\n' '|')"
fi

# --- 16. ADVERSARIAL: capture-file loss fails loud, never emits an empty hash -
# A concurrent validator once removed a generic /tmp/tmp.* capture while the
# child ran. The wrapper then emitted exit 1, lines 0, and a blank sha256 — an
# evidence-shaped result that proved nothing. The child receives only the
# private path so this fixture can reproduce that deletion deterministically.
set +e
missing_out="$(bash "$TARGET" -- bash -c 'rm -f "$BUBBLES_EVIDENCE_CAPTURE_OUTPUT_PATH"' 2>&1)"
missing_rc=$?
set -e
if [[ "$missing_rc" -eq 2 ]] &&
  printf '%s' "$missing_out" | grep -q 'capture output disappeared during command execution' &&
  ! printf '%s' "$missing_out" | grep -q '^sha256:[[:space:]]*$'; then
  ok "capture-file loss fails loud without emitting an empty evidence hash"
else
  bad "capture-file loss fails loud" "rc=$missing_rc $(printf '%s' "$missing_out" | tr '\n' '|')"
fi

# --- 17. SCN-B053-001: empty success emits one scalar zero ------------------
empty_sha256='e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855'
set +e
empty_success_out="$(bash "$TARGET" -- bash -c 'exit 0' 2>&1)"
empty_success_rc=$?
set -e
empty_success_line_fields="$(printf '%s\n' "$empty_success_out" | awk '/^lines: / { count += 1 } END { print count + 0 }')"
if [[ "$empty_success_rc" -eq 0 ]] &&
  printf '%s\n' "$empty_success_out" | grep -qx 'exit: 0' &&
  [[ "$empty_success_line_fields" -eq 1 ]] &&
  printf '%s\n' "$empty_success_out" | grep -qx 'lines: 0' &&
  printf '%s\n' "$empty_success_out" | grep -qx "sha256: $empty_sha256" &&
  ! printf '%s\n' "$empty_success_out" | grep -qx '0' &&
  ! printf '%s\n' "$empty_success_out" | grep -Eqi 'arithmetic|syntax error|operand expected'; then
  ok "SCN-B053-001 empty successful output emits one clean zero count"
else
  bad "SCN-B053-001 empty successful output" "rc=$empty_success_rc lines_fields=$empty_success_line_fields $(printf '%s' "$empty_success_out" | tr '\n' '|')"
fi

# --- 18. SCN-B053-002: empty failure preserves exit seven ------------------
set +e
empty_failure_out="$(bash "$TARGET" -- bash -c 'exit 7' 2>&1)"
empty_failure_rc=$?
set -e
empty_failure_line_fields="$(printf '%s\n' "$empty_failure_out" | awk '/^lines: / { count += 1 } END { print count + 0 }')"
if [[ "$empty_failure_rc" -eq 7 ]] &&
  printf '%s\n' "$empty_failure_out" | grep -qx 'exit: 7' &&
  [[ "$empty_failure_line_fields" -eq 1 ]] &&
  printf '%s\n' "$empty_failure_out" | grep -qx 'lines: 0' &&
  printf '%s\n' "$empty_failure_out" | grep -qx "sha256: $empty_sha256" &&
  ! printf '%s\n' "$empty_failure_out" | grep -qx '0' &&
  ! printf '%s\n' "$empty_failure_out" | grep -Eqi 'arithmetic|syntax error|operand expected'; then
  ok "SCN-B053-002 empty failing output preserves exit seven and clean metadata"
else
  bad "SCN-B053-002 empty failing output" "rc=$empty_failure_rc lines_fields=$empty_failure_line_fields $(printf '%s' "$empty_failure_out" | tr '\n' '|')"
fi

# --- 19. SCN-B053-003: one-line short output remains compatible ------------
set +e
one_line_out="$(bash "$TARGET" -- printf 'kept\n' 2>&1)"
one_line_rc=$?
set -e
one_line_fields="$(printf '%s\n' "$one_line_out" | awk '/^lines: / { count += 1 } END { print count + 0 }')"
if [[ "$one_line_rc" -eq 0 ]] &&
  [[ "$one_line_fields" -eq 1 ]] &&
  printf '%s\n' "$one_line_out" | grep -qx 'lines: 1' &&
  printf '%s\n' "$one_line_out" | grep -qx -- '--- output ---' &&
  printf '%s\n' "$one_line_out" | grep -qx 'kept'; then
  ok "SCN-B053-003 one-line output retains its count and short rendering"
else
  bad "SCN-B053-003 one-line compatibility" "rc=$one_line_rc lines_fields=$one_line_fields $(printf '%s' "$one_line_out" | tr '\n' '|')"
fi

printf '\n%s: %d/%d checks passed\n' "$NAME" "$((checks - failures))" "$checks"
if [[ "$failures" -gt 0 ]]; then
  printf '%s: FAILED\n' "$NAME"
  exit 1
fi
printf '%s: OK\n' "$NAME"
exit 0
