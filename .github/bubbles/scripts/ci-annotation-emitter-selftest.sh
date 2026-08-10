#!/usr/bin/env bash
# ci-annotation-emitter-selftest.sh — hermetic selftest for the GitHub Actions
# `::error::` annotation emitter (OW-002).
#
# WHY THIS EXISTS
# ---------------
# A red macOS `release-hygiene-macos` job could not be attributed to a specific
# failing check from an unprivileged machine: `GET /actions/jobs/<id>/logs`
# requires repo ADMIN and answers 403. Check-run ANNOTATIONS have no such
# requirement — `GET /repos/<owner>/<repo>/check-runs/<id>/annotations` answered
# UNAUTHENTICATED on 2026-08-07. So `bubbles_ci_annotate_failure` emits one
# annotation per failing check, making every future failure attributable with
# ZERO credentials.
#
# The emitter is ADDITIVE (the plain `FAIL: <label>` line other tooling parses
# is untouched) and GATED on GITHUB_ACTIONS, so local runs are unchanged.
#
# HOW THIS AVOIDS BEING TAUTOLOGICAL
# ----------------------------------
# Cases A-E do NOT re-implement run_check. They EXTRACT the real `run_check`
# body from the shipped release-check.sh and eval it against the real
# guard-lib.sh, so deleting the emitter CALL from release-check.sh, or the
# emitter FUNCTION from guard-lib.sh, makes them fail.
#
# Case B (no GITHUB_ACTIONS => no annotation) and Case E (a PASSING check emits
# no annotation) are the assertions that stop this from passing no matter what:
# an emitter that fired unconditionally would satisfy A, C and D but fail B and E.
#
# Case F generalizes: EVERY `echo "FAIL: ` site in the two production scripts
# must be annotated, so a NEW unannotated FAIL site added later is also caught.
#
# NOTE ON OUTPUT: this selftest never prints a raw `::error::` line. Doing so
# would make GitHub attach a bogus annotation to a PASSING run. Diagnostics are
# emitted with the `::` masked.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD_LIB="$SCRIPT_DIR/guard-lib.sh"
RELEASE_CHECK="$SCRIPT_DIR/release-check.sh"
FRAMEWORK_VALIDATE="$SCRIPT_DIR/framework-validate.sh"

failures=0
pass() { printf '  PASS  %s\n' "$1"; }
fail() { printf '  FAIL  %s\n' "$1"; failures=$((failures + 1)); }

# Mask `::` so diagnostics can quote captured output without GitHub parsing it
# as a real workflow command.
masked() { printf '%s' "$1" | sed 's/::/;;/g'; }

for required in "$GUARD_LIB" "$RELEASE_CHECK" "$FRAMEWORK_VALIDATE"; do
  if [[ ! -f "$required" ]]; then
    printf 'ci-annotation-emitter-selftest: FAIL: required file missing: %s\n' "$required" >&2
    exit 1
  fi
done

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT INT TERM

# --- Build a harness around the REAL run_check from release-check.sh ---------
# Extracting the shipped function (rather than restating it) is what gives the
# cases below teeth: remove `bubbles_ci_annotate_failure` from release-check.sh
# and A/C/D stop observing an annotation.
sed -n '/^run_check() {/,/^}/p' "$RELEASE_CHECK" >"$TMP/run_check.body"

if [[ ! -s "$TMP/run_check.body" ]] || ! grep -q 'echo "FAIL: \$label"' "$TMP/run_check.body"; then
  fail "could not extract run_check() from release-check.sh (shape changed?)"
  printf 'ci-annotation-emitter-selftest: %d failure(s)\n' "$failures"
  exit 1
fi

{
  printf '#!/usr/bin/env bash\n'
  printf 'set -uo pipefail\n'
  printf 'source "%s"\n' "$GUARD_LIB"
  printf 'failures=0\n'
  cat "$TMP/run_check.body"
  printf 'run_check "$1" bash -c "exit $2"\n'
  printf 'exit 0\n'
} >"$TMP/harness.sh"

OUT="$TMP/out.txt"

# run_harness <ci|local> <label> <exit-code>
run_harness() {
  local mode="$1" label="$2" rc="$3"
  if [[ "$mode" == "ci" ]]; then
    GITHUB_ACTIONS=true bash "$TMP/harness.sh" "$label" "$rc" >"$OUT" 2>&1
  else
    # Explicitly REMOVE the variable. This selftest itself runs inside GitHub
    # Actions, where GITHUB_ACTIONS is already true, so inheriting it would
    # silently invert Case B.
    env -u GITHUB_ACTIONS bash "$TMP/harness.sh" "$label" "$rc" >"$OUT" 2>&1
  fi
}

annotation_count() { grep -c '^::error::' "$OUT" 2>/dev/null || true; }
annotation_line() { grep '^::error::' "$OUT" 2>/dev/null | head -n1 || true; }

LABEL='Release manifest freshness'

# --- Case A: failing check under GITHUB_ACTIONS=true emits a naming annotation
run_harness ci "$LABEL" 1
count="$(annotation_count)"
line="$(annotation_line)"
if [[ "$count" == "1" ]] && printf '%s' "$line" | grep -Fq "$LABEL"; then
  pass "GITHUB_ACTIONS=true: failing check emits 1 annotation naming the check"
else
  fail "expected 1 annotation naming '$LABEL', got count=$count line=[$(masked "$line")]"
fi

# --- Case C1: the plain FAIL line survives in CI mode (additive, not replacing)
if grep -Fq "FAIL: $LABEL" "$OUT"; then
  pass "GITHUB_ACTIONS=true: plain 'FAIL: <label>' line still emitted"
else
  fail "plain 'FAIL: $LABEL' line missing under GITHUB_ACTIONS=true"
fi

# --- Case B: NO GITHUB_ACTIONS => NO annotation (local output unchanged) ------
run_harness local "$LABEL" 1
count="$(annotation_count)"
if [[ "$count" == "0" ]]; then
  pass "GITHUB_ACTIONS unset: no annotation emitted (local output unchanged)"
else
  fail "expected 0 annotations without GITHUB_ACTIONS, got $count: [$(masked "$(annotation_line)")]"
fi

# --- Case C2: the plain FAIL line is still emitted locally --------------------
if grep -Fq "FAIL: $LABEL" "$OUT"; then
  pass "GITHUB_ACTIONS unset: plain 'FAIL: <label>' line still emitted"
else
  fail "plain 'FAIL: $LABEL' line missing with GITHUB_ACTIONS unset"
fi

# --- Case E (adversarial): a PASSING check emits NO annotation, even in CI ----
# Without this, an emitter that fired on every check would still pass A/C/D.
run_harness ci "$LABEL" 0
count="$(annotation_count)"
if [[ "$count" == "0" ]] && grep -Fq "PASS: $LABEL" "$OUT"; then
  pass "GITHUB_ACTIONS=true: passing check emits no annotation"
else
  fail "passing check emitted $count annotation(s) or lost its PASS line"
fi

# --- Case D (adversarial): '%' and newline are escaped, raw form absent -------
# A raw newline would split the workflow command across two lines (truncating
# it and allowing a forged second command); a raw '%' corrupts the payload.
ADV_LABEL='pct-100%-done
second-line'
run_harness ci "$ADV_LABEL" 1
count="$(annotation_count)"
line="$(annotation_line)"

if [[ "$count" == "1" ]]; then
  pass "escaping: embedded newline did not split the annotation (exactly 1 line)"
else
  fail "expected exactly 1 annotation line for a multi-line label, got $count"
fi

if printf '%s' "$line" | grep -Fq 'pct-100%25-done%0Asecond-line'; then
  pass "escaping: '%' -> %25 and LF -> %0A both applied"
else
  fail "expected escaped 'pct-100%25-done%0Asecond-line', got [$(masked "$line")]"
fi

# The raw, unescaped percent sequence must NOT survive into the annotation.
if printf '%s' "$line" | grep -Fq 'pct-100%-done'; then
  fail "raw unescaped '%' leaked into the annotation: [$(masked "$line")]"
else
  pass "escaping: raw unescaped '%' form absent from the annotation"
fi

# Double-escaping regression: '%' must be substituted BEFORE %0D/%0A, otherwise
# the '%' those introduce is itself re-escaped into %250A.
if printf '%s' "$line" | grep -Fq '%250'; then
  fail "double-escaped sequence '%250' found; '%' was substituted after CR/LF"
else
  pass "escaping: no double-escaped '%250' sequence (substitution order correct)"
fi

# The plain FAIL line keeps the label VERBATIM (escaping is annotation-only).
if grep -Fq 'FAIL: pct-100%-done' "$OUT"; then
  pass "escaping: plain FAIL line keeps the label verbatim (unescaped)"
else
  fail "plain FAIL line lost the verbatim label"
fi

# --- Case F: every FAIL site in the production scripts is annotated -----------
# Generalizing guard: catches a NEW `echo "FAIL: ` added later without an
# annotation, and catches removal of any existing emitter call.
assert_fail_sites_annotated() {
  local script="$1" name="$2" missing=0 total=0 lineno
  while IFS= read -r lineno; do
    [[ -n "$lineno" ]] || continue
    total=$((total + 1))
    if ! sed -n "$((lineno + 1)),$((lineno + 6))p" "$script" | grep -q 'bubbles_ci_annotate_failure'; then
      missing=$((missing + 1))
      printf '        unannotated FAIL site: %s:%s\n' "$name" "$lineno"
    fi
  done < <(grep -n 'echo "FAIL: ' "$script" | cut -d: -f1)

  if [[ "$total" -eq 0 ]]; then
    fail "$name: found no 'echo \"FAIL: ' site to check (shape changed?)"
    return
  fi
  if [[ "$missing" -eq 0 ]]; then
    pass "$name: all $total FAIL site(s) emit an annotation"
  else
    fail "$name: $missing of $total FAIL site(s) emit no annotation"
  fi
}

assert_fail_sites_annotated "$FRAMEWORK_VALIDATE" "framework-validate.sh"
assert_fail_sites_annotated "$RELEASE_CHECK" "release-check.sh"

# --- Case G: release-check.sh actually sources the lib providing the emitter --
if grep -q 'source "\$SCRIPT_DIR/guard-lib.sh"' "$RELEASE_CHECK"; then
  pass "release-check.sh sources guard-lib.sh (emitter is in scope)"
else
  fail "release-check.sh does not source guard-lib.sh; the emitter would be unbound"
fi

# --- Case H: the emitter is gated on GITHUB_ACTIONS, not on an ad-hoc flag ----
if grep -q 'GITHUB_ACTIONS' "$GUARD_LIB"; then
  pass "emitter gates on GitHub's own GITHUB_ACTIONS signal"
else
  fail "guard-lib.sh does not reference GITHUB_ACTIONS"
fi

# --- Case I: bubbles_ci_failure_detail (OW-002 phase 2) ----------------------
# Naming WHICH check failed is not enough to diagnose a macOS-only failure when
# the raw job log is 403 admin-only. The annotation must also carry WHY, so
# run_check captures the check's output under CI and feeds the failure-shaped
# lines to the annotation.
#
# The harness runs under `set -euo pipefail` — the SAME regime as
# framework-validate.sh — not under this selftest's laxer `set -uo pipefail`.
# That is what gives I5/I6 teeth: a helper whose pipeline returns 1 when grep
# matches nothing aborts its caller on exactly the "fall back to the bare
# label" path the helper documents, and an inline call here would not notice.
{
  printf '#!/usr/bin/env bash\n'
  printf 'set -euo pipefail\n'
  printf 'source "%s"\n' "$GUARD_LIB"
  printf 'bubbles_ci_failure_detail "$1"\n'
  printf 'printf "HARNESS-SURVIVED\\n"\n'
} >"$TMP/detail-harness.sh"

# run_detail <fixture-path>  -> stdout+stderr in $OUT, exit status in $DETAIL_RC
run_detail() {
  bash "$TMP/detail-harness.sh" "$1" >"$OUT" 2>&1
  DETAIL_RC=$?
}

# detail_lines: emitted lines EXCLUDING the harness survival marker.
detail_lines() { grep -cv '^HARNESS-SURVIVED$' "$OUT" 2>/dev/null || true; }

# Fixture 1: a real-shaped log interleaving passing and failing lines.
{
  printf 'PASS: sentinel-should-not-appear\n'
  printf 'FAIL: sentinel-assertion-mismatch\n'
  printf 'PASS: another-green-line\n'
  printf 'ERROR: connection refused\n'
} >"$TMP/mixed.log"

run_detail "$TMP/mixed.log"

# --- I1: the FAIL line is surfaced -------------------------------------------
if grep -Fq 'FAIL: sentinel-assertion-mismatch' "$OUT"; then
  pass "failure_detail: surfaces the failing assertion line from a mixed log"
else
  fail "failure_detail: expected the FAIL line, got [$(masked "$(cat "$OUT")")]"
fi

# --- I2 (adversarial): PASS noise is NOT surfaced ----------------------------
# Without this, a helper that simply `cat`ed the whole log would satisfy I1.
if grep -Fq 'PASS: sentinel-should-not-appear' "$OUT"; then
  fail "failure_detail: leaked a PASS line into the detail body"
else
  pass "failure_detail: passing lines absent from the detail body"
fi

# --- I3: output is capped at 10 lines ----------------------------------------
# An uncapped body would blow past GitHub's annotation size limit and could
# push the useful first line out of view.
: >"$TMP/many.log"
for i in {1..25}; do
  printf 'FAIL: assertion number %s\n' "$i" >>"$TMP/many.log"
done
run_detail "$TMP/many.log"
lines="$(detail_lines)"
if [[ "$lines" == "10" ]]; then
  pass "failure_detail: caps output at 10 lines given 25 failure lines"
else
  fail "failure_detail: expected 10 lines from a 25-line failure log, got $lines"
fi

# --- I4: a log with no failure-shaped line yields NO detail -------------------
# The caller keys off empty output to fall back to the bare label.
{
  printf 'PASS: everything is fine\n'
  printf 'ok 1 - nothing to see here\n'
} >"$TMP/clean.log"
run_detail "$TMP/clean.log"
lines="$(detail_lines)"
if [[ "$lines" == "0" ]]; then
  pass "failure_detail: emits nothing when the log has no failure-shaped line"
else
  fail "failure_detail: expected no detail for a clean log, got [$(masked "$(cat "$OUT")")]"
fi

# --- I5 (adversarial): the clean-log path must not abort a set -e caller ------
# This is the regression that makes I4 meaningful. `grep | head | cut` returns 1
# under pipefail when grep matches nothing; framework-validate.sh assigns the
# result as the final command of an `&&` list, which `set -e` does NOT exempt.
# Without a guaranteed 0 return, framework-validate would die mid-run on the
# fallback path instead of annotating the bare label.
if [[ "$DETAIL_RC" -eq 0 ]] && grep -Fq 'HARNESS-SURVIVED' "$OUT"; then
  pass "failure_detail: clean log returns 0 and does not abort a set -euo pipefail caller"
else
  fail "failure_detail: clean log aborted the set -e caller (rc=$DETAIL_RC, survived=$(grep -Fc 'HARNESS-SURVIVED' "$OUT" 2>/dev/null || true))"
fi

# --- I6: a non-existent path is silent and non-fatal under set -e -------------
run_detail "$TMP/definitely-not-a-real-file.log"
lines="$(detail_lines)"
if [[ "$DETAIL_RC" -eq 0 ]] && [[ "$lines" == "0" ]] && grep -Fq 'HARNESS-SURVIVED' "$OUT"; then
  pass "failure_detail: missing file emits nothing and does not error under set -e"
else
  fail "failure_detail: missing file misbehaved (rc=$DETAIL_RC, lines=$lines)"
fi

# --- Case I7/I8: run_check actually WIRES the detail into the annotation ------
# Generalizing static guard. I1-I6 prove the helper works; these prove
# framework-validate.sh still USES it. A future edit that drops the capture or
# the detail call would leave I1-I6 green while silently restoring the
# label-only annotation this work exists to replace.
sed -n '/^run_check() {/,/^}/p' "$FRAMEWORK_VALIDATE" >"$TMP/fv_run_check.body"

if [[ ! -s "$TMP/fv_run_check.body" ]]; then
  fail "could not extract run_check() from framework-validate.sh (shape changed?)"
else
  if grep -Fq 'bubbles_ci_failure_detail' "$TMP/fv_run_check.body"; then
    pass "framework-validate.sh run_check feeds bubbles_ci_failure_detail into the annotation"
  else
    fail "framework-validate.sh run_check no longer calls bubbles_ci_failure_detail"
  fi

  if grep -Fq 'GITHUB_ACTIONS' "$TMP/fv_run_check.body" &&
    grep -Fq 'tee "$_cap"' "$TMP/fv_run_check.body"; then
    pass "framework-validate.sh run_check captures output behind a GITHUB_ACTIONS gate"
  else
    fail "framework-validate.sh run_check lost the GITHUB_ACTIONS-gated 'tee \"\$_cap\"' capture"
  fi
fi

# --- Case I9-I17: TOOL/INTERPRETER errors must be surfaced, not filtered out --
# WHY (measured). A macOS check failed with
#     awk: line 27: syntax error at or near ,
# That line starts with none of FAIL/ERROR/AssertionError/Traceback/not ok, so
# the shape-1-only extractor dropped it. The annotation then carried only failed
# assertions with empty values, which made a CRASHED INTERPRETER look like code
# that runs but produces wrong content — a mis-signal that cost a long
# investigation. I9-I12 are the regression guards for that exact failure mode.
{
  printf 'PASS: green line that must not appear\n'
  printf 'awk: line 27: syntax error at or near ,\n'
  printf 'jq: error (at <stdin>:0): null (null) has no keys\n'
  printf 'bash: line 3: foo: command not found\n'
  printf '/usr/lib/thing.sh: line 9: MYVAR: unbound variable\n'
  printf 'sed: -i requires an argument\n'
  printf 'ok 2 - benign\n'
} >"$TMP/toolerr.log"
run_detail "$TMP/toolerr.log"

# --- I9: the awk parse error (the measured mis-signal) is surfaced ------------
if grep -Fq 'awk: line 27: syntax error at or near ,' "$OUT"; then
  pass "failure_detail: surfaces an awk interpreter parse error"
else
  fail "failure_detail: dropped the awk parse error, got [$(masked "$(cat "$OUT")")]"
fi

# --- I10: a jq tool error is surfaced ----------------------------------------
if grep -Fq 'jq: error (at <stdin>:0): null' "$OUT"; then
  pass "failure_detail: surfaces a jq tool error"
else
  fail "failure_detail: dropped the jq tool error"
fi

# --- I11: a shell 'command not found' is surfaced ----------------------------
if grep -Fq 'bash: line 3: foo: command not found' "$OUT"; then
  pass "failure_detail: surfaces a shell 'command not found'"
else
  fail "failure_detail: dropped the 'command not found' line"
fi

# --- I12: the phrase branch fires on a line whose first token is NOT a tool ---
# `/usr/lib/thing.sh:` is not in the tool alternation, so only the unanchored
# phrase branch can catch it. Without that branch this assertion goes red.
if grep -Fq '/usr/lib/thing.sh: line 9: MYVAR: unbound variable' "$OUT"; then
  pass "failure_detail: phrase branch catches an error on a non-tool-prefixed line"
else
  fail "failure_detail: dropped an 'unbound variable' line lacking a tool prefix"
fi

# --- I13 (adversarial): benign lines in the SAME log are still NOT surfaced ---
# Without this, widening the regex all the way to `cat` would satisfy I9-I12.
leaked=''
grep -Fq 'PASS: green line that must not appear' "$OUT" && leaked="$leaked [PASS line]"
grep -Fq 'ok 2 - benign' "$OUT" && leaked="$leaked [ok line]"
if [[ -z "$leaked" ]]; then
  pass "failure_detail: benign lines absent from a log full of tool errors"
else
  fail "failure_detail: widened regex leaked benign lines:$leaked"
fi

# --- I14: every pre-existing assertion shape still matches (regression guard) -
# The tool/phrase branches are ADDITIVE. If shape 1 were lost, this goes red
# while I9-I12 stay green.
{
  printf 'Traceback (most recent call last):\n'
  printf 'AssertionError: expected 1 got 0\n'
  printf 'not ok 3 - legacy tap failure\n'
  printf 'ERROR: legacy error line\n'
  printf 'FAIL: legacy fail line\n'
  printf '✗ legacy cross marker\n'
  printf '❌ legacy emoji marker\n'
  printf 'PASS: legacy green line\n'
} >"$TMP/legacy.log"
run_detail "$TMP/legacy.log"
legacy_missing=''
while IFS= read -r want; do
  grep -Fq "$want" "$OUT" || legacy_missing="$legacy_missing [$want]"
done <<'LEGACY_SHAPES'
Traceback (most recent call last):
AssertionError: expected 1 got 0
not ok 3 - legacy tap failure
ERROR: legacy error line
FAIL: legacy fail line
✗ legacy cross marker
❌ legacy emoji marker
LEGACY_SHAPES
if [[ -z "$legacy_missing" ]]; then
  pass "failure_detail: all 7 pre-existing assertion shapes still captured"
else
  fail "failure_detail: lost pre-existing shape(s):$legacy_missing"
fi

# --- I15 (adversarial): zero exit under pipefail when NOTHING matches ---------
# This is the pipefail landmine restated for the WIDENED regex. Every line below
# is a deliberate NEAR MISS: `shellcheck:`/`python-ish:`/`sorted:` prove the tool
# branch requires the colon immediately after the token, `failures:` proves shape
# 1 stayed case-sensitive, `notok` proves `not ok` still needs its space. So grep
# matches nothing, returns 1, pipefail propagates it — and only the load-bearing
# `|| true` keeps the set -euo pipefail harness alive.
{
  printf 'PASS: everything nominal\n'
  printf 'shellcheck: no issues found\n'
  printf 'python-ish: fine\n'
  printf 'sorted: yes\n'
  printf 'notok 4 - hyphenless\n'
  printf 'failures: 0\n'
} >"$TMP/nearmiss.log"
run_detail "$TMP/nearmiss.log"
lines="$(detail_lines)"
if [[ "$DETAIL_RC" -eq 0 ]] && [[ "$lines" == "0" ]] && grep -Fq 'HARNESS-SURVIVED' "$OUT"; then
  pass "failure_detail: near-miss log matches nothing AND returns 0 under set -euo pipefail"
else
  fail "failure_detail: near-miss log misbehaved (rc=$DETAIL_RC, lines=$lines, expected rc=0 lines=0)"
fi

# --- I16: the 10-line cap still holds for the widened regex -------------------
: >"$TMP/many-tool.log"
for i in {1..25}; do
  printf 'awk: line %s: syntax error at or near ,\n' "$i" >>"$TMP/many-tool.log"
done
run_detail "$TMP/many-tool.log"
lines="$(detail_lines)"
if [[ "$lines" == "10" ]]; then
  pass "failure_detail: caps output at 10 lines given 25 tool-error lines"
else
  fail "failure_detail: expected 10 lines from a 25-line tool-error log, got $lines"
fi

# --- I17: a long line is still truncated to 300 chars -------------------------
# An annotation body is size-limited; one runaway line must not consume it.
long_tail=''
while [[ "${#long_tail}" -lt 400 ]]; do
  long_tail="${long_tail}0123456789"
done
printf 'awk: syntax error %s\n' "$long_tail" >"$TMP/longline.log"
run_detail "$TMP/longline.log"
longest=0
while IFS= read -r captured; do
  [[ "$captured" == "HARNESS-SURVIVED" ]] && continue
  if [[ "${#captured}" -gt "$longest" ]]; then
    longest="${#captured}"
  fi
done <"$OUT"
if [[ "$longest" -eq 300 ]]; then
  pass "failure_detail: truncates an over-long tool-error line to 300 chars"
else
  fail "failure_detail: expected a 300-char cap, longest captured line was $longest"
fi

if [[ "$failures" -ne 0 ]]; then
  printf 'ci-annotation-emitter selftest: %d failure(s)\n' "$failures"
  exit 1
fi
printf 'ci-annotation-emitter selftest: OK (31 assertions)\n'
exit 0
