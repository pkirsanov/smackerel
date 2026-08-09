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

if [[ "$failures" -ne 0 ]]; then
  printf 'ci-annotation-emitter selftest: %d failure(s)\n' "$failures"
  exit 1
fi
printf 'ci-annotation-emitter selftest: OK (14 assertions)\n'
exit 0
