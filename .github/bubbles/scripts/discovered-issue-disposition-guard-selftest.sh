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
#   S7   Report-and-wait phrase ("recommend filing")     → exit 1  (ADVERSARIAL:
#        with no disposition                              reported-but-unfiled)
#   S8   Report-and-wait phrase + inline BUG-NNN         → exit 0
#   S13  Forbidden phrase INSIDE a fenced block        → exit 0  (REGRESSION:
#        (verbatim evidence capture)                             evidence is
#                                                                not deferral)
#   S14  Fenced evidence + an out-of-fence deferral    → exit 1  (ADVERSARIAL:
#                                                                fence fix did
#                                                                not relax the
#                                                                gate)
#   S15  Unterminated fence, deferral after it         → exit 1  (ADVERSARIAL:
#                                                                scan-on-doubt,
#                                                                no false neg)
#   S16  Indented fence delimiters                     → exit 0
#   S17  Finding cites real report.md path + true line → exit 1
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
# S7: report-and-wait phrase ("recommend filing"), no disposition → exit 1
#     (ADVERSARIAL: proves the gate catches a reported-but-UNFILED finding —
#      the exact anti-pattern of handing the user a findings list to authorize)
# -----------------------------------------------------------------------
s7="$(new_spec_dir s7-report-and-wait)"
cat >"$s7/report.md" <<EOF
# Report

## Test Evidence

Found an in-memory persistence gap. Recommend filing these as bugs in a follow-up workflow.
EOF
run_guard "$s7"
if [[ "$RC" -eq 1 ]]; then
  pass "S7 'recommend filing' report-and-wait without disposition BLOCKs (exit 1)"
else
  bad "S7 report-and-wait deferral expected exit 1, got $RC"
fi

# -----------------------------------------------------------------------
# S8: report-and-wait phrase + inline BUG-NNN citation → exit 0
#     (proves "recommend filing" PAIRED with a filed artifact passes)
# -----------------------------------------------------------------------
s8="$(new_spec_dir s8-filed-now)"
cat >"$s8/report.md" <<EOF
# Report

## Test Evidence

Found an in-memory persistence gap. Recommend filing as a bug — done: filed as BUG-027 (bug-filed).
EOF
run_guard "$s8"
if [[ "$RC" -eq 0 ]]; then
  pass "S8 report-and-wait phrase with inline BUG-NNN citation passes (exit 0)"
else
  bad "S8 filed-now expected exit 0, got $RC"
fi

# -----------------------------------------------------------------------
# S9: pre-marker forbidden phrase is frozen prior-window history → exit 0
#     (certifying-window boundary, report.md marker parity with
#      artifact-lint.sh Check 3; opt-in prior-window exemption).
# -----------------------------------------------------------------------
s9="$(new_spec_dir s9-cw-pre-marker-skipped)"
cat >"$s9/report.md" <<'EOF'
# Report

## Prior-Window History

An earlier round left the currency rounding bug out of scope for that session.

<!-- bubbles:certifying-window-begin -->

## Current Window

All current-window findings are dispositioned.
EOF
run_guard "$s9"
if [[ "$RC" -eq 0 ]]; then
  pass "S9 pre-marker 'out of scope' is frozen prior-window history (exit 0)"
else
  bad "S9 pre-marker deferral expected exit 0, got $RC"
fi

# -----------------------------------------------------------------------
# S10: the SAME phrase AFTER the marker still BLOCKs → exit 1
#      (ADVERSARIAL: current-window strictness intact).
# -----------------------------------------------------------------------
s10="$(new_spec_dir s10-cw-post-marker-blocks)"
cat >"$s10/report.md" <<'EOF'
# Report

## Prior-Window History

All prior rounds are complete.

<!-- bubbles:certifying-window-begin -->

## Current Window

The currency rounding bug is out of scope for this session, so we left it.
EOF
run_guard "$s10"
if [[ "$RC" -eq 1 ]]; then
  pass "S10 post-marker 'out of scope' without disposition still BLOCKs (exit 1)"
else
  bad "S10 post-marker deferral expected exit 1, got $RC"
fi

# -----------------------------------------------------------------------
# S11: a marker-less report.md is enforced in FULL → exit 1
#      (ADVERSARIAL: the marker can never silently disable G095).
# -----------------------------------------------------------------------
s11="$(new_spec_dir s11-cw-no-marker)"
cat >"$s11/report.md" <<'EOF'
# Report

## Test Evidence

The currency rounding bug is out of scope for this session, so we left it.
EOF
run_guard "$s11"
if [[ "$RC" -eq 1 ]]; then
  pass "S11 marker-less report is enforced in FULL (exit 1)"
else
  bad "S11 marker-less deferral expected exit 1, got $RC"
fi

# -----------------------------------------------------------------------
# S12: TWO certifying-window markers are ambiguous → exit 2 (fail loud).
# -----------------------------------------------------------------------
s12="$(new_spec_dir s12-cw-two-markers)"
cat >"$s12/report.md" <<'EOF'
# Report

## Prior-Window History

The currency rounding bug is out of scope for this session.

<!-- bubbles:certifying-window-begin -->

<!-- bubbles:certifying-window-begin -->

## Current Window

Done.
EOF
run_guard "$s12"
if [[ "$RC" -eq 2 ]]; then
  pass "S12 two certifying-window markers exit 2 (ambiguous window start)"
else
  bad "S12 two-marker report expected exit 2, got $RC"
fi

# -----------------------------------------------------------------------
# S13: forbidden phrase INSIDE a fenced code block → exit 0
#      (REGRESSION: the Execution Evidence Standard requires verbatim terminal
#       captures in report.md. Quoting the guard's own BLOCK line — or a grep
#       hit — is EVIDENCE, not a deferral. Before the fence fix, documenting a
#       G095 finding manufactured the next one.)
# -----------------------------------------------------------------------
s13="$(new_spec_dir s13-fenced-evidence)"
cat >"$s13/report.md" <<'EOF'
# Report

## Test Evidence

Finding AUD-F3 was RESOLVED via the in-paragraph-citation path; see BUG-003.

Verbatim guard capture:

```text
🔴 G095 BLOCK: report.md:3198 — forbidden deferral phrase 'out of scope' without disposition citation
```

Verbatim grep capture:

```text
3205:  the panel was out of scope for that round
```
EOF
run_guard "$s13"
if [[ "$RC" -eq 0 ]]; then
  pass "S13 forbidden phrase inside a fenced block is evidence, not deferral (exit 0)"
else
  bad "S13 fenced-evidence expected exit 0, got $RC"
fi

# -----------------------------------------------------------------------
# S14: the SAME fenced evidence PLUS an out-of-fence deferral → exit 1
#      (ADVERSARIAL: proves fence stripping is narrowly scoped and did NOT
#       blanket-disable detection. Fails if the fix over-reaches.)
# -----------------------------------------------------------------------
s14="$(new_spec_dir s14-fence-plus-real-deferral)"
cat >"$s14/report.md" <<'EOF'
# Report

## Test Evidence

Verbatim guard capture:

```text
🔴 G095 BLOCK: report.md:3198 — forbidden deferral phrase 'out of scope'
```

The currency rounding bug is out of scope for this session, so we left it.
EOF
run_guard "$s14"
if [[ "$RC" -eq 1 ]]; then
  pass "S14 out-of-fence deferral still BLOCKs alongside fenced evidence (exit 1)"
else
  bad "S14 fence+real-deferral expected exit 1, got $RC"
fi

# -----------------------------------------------------------------------
# S15: UNBALANCED (unterminated) fence, deferral after it → exit 1
#      (ADVERSARIAL: a stray fence must never silently disable scanning for the
#       rest of the file. Fail-safe toward detection — the unterminated region
#       is scanned VERBATIM. A false negative here is far worse than a false
#       positive, so scan-on-doubt is the contract.)
# -----------------------------------------------------------------------
s15="$(new_spec_dir s15-unbalanced-fence)"
cat >"$s15/report.md" <<'EOF'
# Report

## Test Evidence

```text
a stray opening fence that is never closed

The currency rounding bug is out of scope for this session, so we left it.
EOF
run_guard "$s15"
if [[ "$RC" -eq 1 ]]; then
  pass "S15 unterminated fence still scans the remainder — deferral BLOCKs (exit 1)"
else
  bad "S15 unbalanced-fence expected exit 1, got $RC"
fi

# -----------------------------------------------------------------------
# S16: INDENTED fence delimiters are honored → exit 0
#      (parity with the certifying-window helper's /^[[:space:]]*```/ pattern)
# -----------------------------------------------------------------------
s16="$(new_spec_dir s16-indented-fence)"
cat >"$s16/report.md" <<'EOF'
# Report

## Test Evidence

All assertions green. Nested list evidence:

  ```text
  the panel was out of scope for that round
  ```
EOF
run_guard "$s16"
if [[ "$RC" -eq 0 ]]; then
  pass "S16 indented fence delimiters are honored (exit 0)"
else
  bad "S16 indented fence expected exit 0, got $RC"
fi

# -----------------------------------------------------------------------
# S17: finding cites the REAL report.md path and TRUE line number → exit 1
#      (suppressed regions are blanked, not deleted, so `path:line` stays
#       actionable even when a Discovered Issues table precedes the narrative)
# -----------------------------------------------------------------------
s17="$(new_spec_dir s17-line-fidelity)"
cat >"$s17/report.md" <<EOF
# Report

## Discovered Issues

| Observed | Disposition | Reference |
|----------|-------------|-----------|
| $YESTERDAY | bug-filed | BUG-001 |

## Narrative

The currency rounding bug is out of scope for this session, so we left it.
EOF
# The offending narrative is on line 11 of the real report.md.
s17_out="$(bash "$GUARD_SCRIPT" "$s17" 2>&1)"
if grep -qF "$s17/report.md:11" <<<"$s17_out"; then
  pass "S17 finding cites the real report.md path at its true line (11)"
else
  bad "S17 expected finding at '$s17/report.md:11', got: $(grep -m1 'G095 BLOCK' <<<"$s17_out")"
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
