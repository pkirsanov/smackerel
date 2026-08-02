#!/usr/bin/env bash
# technical-prose-lint-selftest.sh — hermetic cases for technical-prose-lint.sh.
#
# The four adversarial cases (A1-A4) are the reason this selftest exists. The
# highest-severity risk in IMP-030 is a prose lint that flags VERBATIM EVIDENCE
# and induces an agent to reword captured terminal output. Each excluded
# surface therefore gets a case that would fail loudly if the exclusion broke.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINT="$SCRIPT_DIR/technical-prose-lint.sh"

FAILURES=0
pass() { printf 'PASS: %s\n' "$1"; }
fail() { printf 'FAIL: %s\n' "$1"; FAILURES=$((FAILURES + 1)); }

TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/technical-prose-selftest.XXXXXX")"
trap 'rm -rf "$TMP_ROOT"' EXIT

OUT=""
RC=0
run_lint() {
  OUT="$("$LINT" "$@" 2>&1)"
  RC=$?
}

new_doc() {
  # $1 = name, stdin = content
  local dir="$TMP_ROOT/$1"
  mkdir -p "$dir"
  cat > "$dir/doc.md"
  printf '%s\n' "$dir/doc.md"
}

# --------------------------------------------------------------------------
# S1 — no surface is a usage error, not a silent pass.
run_lint
if [[ "$RC" -eq 2 ]] && printf '%s' "$OUT" | grep -q 'no scan surface given'; then
  pass "S1 missing surface is a usage error"
else
  fail "S1 expected rc 2 with 'no scan surface given', got rc=$RC: $OUT"
fi

# --------------------------------------------------------------------------
# S2 — a clean document reports zero of everything and exits 0.
doc="$(new_doc s2 <<'MD'
Run the guard before the transition.

The guard reads the manifest and reports one record per file.
MD
)"
run_lint "$doc"
if [[ "$RC" -eq 0 ]] \
  && printf '%s' "$OUT" | grep -q 'over-long-sentence:  *0' \
  && printf '%s' "$OUT" | grep -q 'prose-semicolon:  *0'; then
  pass "S2 clean prose reports zero defects, exit 0"
else
  fail "S2 expected all-zero report, got rc=$RC: $OUT"
fi

# --------------------------------------------------------------------------
# S3 — an over-long sentence is reported.
doc="$(new_doc s3 <<'MD'
This sentence exists only to exceed the configured ceiling and it keeps going
well past the point where a reader can hold the whole instruction in mind at
once which is exactly the defect being detected here today.
MD
)"
run_lint "$doc"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -q 'over-long-sentence:  *1'; then
  pass "S3 over-long sentence reported"
else
  fail "S3 expected one over-long sentence, got rc=$RC: $OUT"
fi

# --------------------------------------------------------------------------
# S4 — a prose semicolon is reported.
doc="$(new_doc s4 <<'MD'
Run the guard; then read the report.
MD
)"
run_lint "$doc"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -q 'prose-semicolon:  *1'; then
  pass "S4 prose semicolon reported"
else
  fail "S4 expected one prose semicolon, got rc=$RC: $OUT"
fi

# --------------------------------------------------------------------------
# A1 (ADVERSARIAL) — a fenced evidence block must be invisible to every rule.
# This is the corruption risk: captured terminal output is verbatim and must
# never be reworded to satisfy a form rule.
doc="$(new_doc a1 <<'MD'
The run produced this evidence.

```
$ cargo test --all; echo "exit=$?"
running 412 tests; 0 failed; 0 ignored; this line is deliberately far longer than any configured ceiling so that a broken exclusion would be impossible to miss in the report
exit=0
```

The suite passed.
MD
)"
run_lint "$doc"
if [[ "$RC" -eq 0 ]] \
  && printf '%s' "$OUT" | grep -q 'over-long-sentence:  *0' \
  && printf '%s' "$OUT" | grep -q 'prose-semicolon:  *0'; then
  pass "A1 fenced evidence block excluded from every rule"
else
  fail "A1 evidence block leaked into the report: rc=$RC: $OUT"
fi

# --------------------------------------------------------------------------
# A2 (ADVERSARIAL) — a markdown table must be excluded.
doc="$(new_doc a2 <<'MD'
The table below is generated.

| Capability | Surface | Status | Note |
|---|---|---|---|
| Restore drill | `cliCommand` | delivered | this cell is long on purpose and carries a semicolon; it must not be reported because a generated table is not prose at all |
MD
)"
run_lint "$doc"
if [[ "$RC" -eq 0 ]] \
  && printf '%s' "$OUT" | grep -q 'over-long-sentence:  *0' \
  && printf '%s' "$OUT" | grep -q 'prose-semicolon:  *0'; then
  pass "A2 markdown table excluded"
else
  fail "A2 table leaked into the report: rc=$RC: $OUT"
fi

# --------------------------------------------------------------------------
# A3 (ADVERSARIAL) — an inline code span must be excluded.
doc="$(new_doc a3 <<'MD'
Set `PATH="$a"; export PATH` before the run.
MD
)"
run_lint "$doc"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -q 'prose-semicolon:  *0'; then
  pass "A3 inline code span excluded"
else
  fail "A3 inline code leaked into the report: rc=$RC: $OUT"
fi

# --------------------------------------------------------------------------
# A4 (ADVERSARIAL) — a blockquoted evidence excerpt must be excluded.
doc="$(new_doc a4 <<'MD'
The operator pasted this.

> FAIL: guard rejected the transition; evidence missing for DoD item 3 and the message continued at considerable length beyond any ceiling this tool applies
MD
)"
run_lint "$doc"
if [[ "$RC" -eq 0 ]] \
  && printf '%s' "$OUT" | grep -q 'over-long-sentence:  *0' \
  && printf '%s' "$OUT" | grep -q 'prose-semicolon:  *0'; then
  pass "A4 blockquoted evidence excluded"
else
  fail "A4 blockquote leaked into the report: rc=$RC: $OUT"
fi

# --------------------------------------------------------------------------
# S5 — the em dash is PERMITTED and must never be reported.
doc="$(new_doc s5 <<'MD'
The guard reads the manifest — then it reports one record per file.
MD
)"
run_lint "$doc"
if [[ "$RC" -eq 0 ]] && ! printf '%s' "$OUT" | grep -qi 'dash'; then
  pass "S5 em dash is never reported"
else
  fail "S5 em dash was reported: rc=$RC: $OUT"
fi

# --------------------------------------------------------------------------
# S6 — no marketing-adjective rule exists.
doc="$(new_doc s6 <<'MD'
This seamless and robust approach will leverage a powerful, innovative,
cutting-edge capability to streamline and enhance the holistic experience.
MD
)"
run_lint "$doc"
if [[ "$RC" -eq 0 ]] \
  && ! printf '%s' "$OUT" | grep -qi 'marketing' \
  && ! printf '%s' "$OUT" | grep -qi 'seamless'; then
  pass "S6 no marketing-adjective blacklist exists"
else
  fail "S6 a marketing check appeared: rc=$RC: $OUT"
fi

# --------------------------------------------------------------------------
# S7 — a phrasal verb is reported.
doc="$(new_doc s7 <<'MD'
Kick off the run and look into the failure.
MD
)"
run_lint "$doc"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -q 'phrasal-verb:  *2'; then
  pass "S7 phrasal verbs reported"
else
  fail "S7 expected two phrasal verbs, got rc=$RC: $OUT"
fi

# --------------------------------------------------------------------------
# S8 — a nominalisation is reported.
doc="$(new_doc s8 <<'MD'
Perform validation of the manifest before the run.
MD
)"
run_lint "$doc"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -q 'nominalisation:  *1'; then
  pass "S8 nominalisation reported"
else
  fail "S8 expected one nominalisation, got rc=$RC: $OUT"
fi

# --------------------------------------------------------------------------
# S9 — a directory surface scans its markdown files.
mkdir -p "$TMP_ROOT/s9/nested"
printf 'Run the guard; read the report.\n' > "$TMP_ROOT/s9/nested/a.md"
printf 'Run the guard first.\n' > "$TMP_ROOT/s9/b.md"
printf 'not markdown; ignored\n' > "$TMP_ROOT/s9/c.txt"
run_lint "$TMP_ROOT/s9"
if [[ "$RC" -eq 0 ]] \
  && printf '%s' "$OUT" | grep -q 'files scanned:  *2' \
  && printf '%s' "$OUT" | grep -q 'prose-semicolon:  *1'; then
  pass "S9 directory surface scans markdown only"
else
  fail "S9 expected 2 files and 1 semicolon, got rc=$RC: $OUT"
fi

# --------------------------------------------------------------------------
# S10 — bypass flags are refused; there is nothing to bypass.
for flag in --skip --force --ignore; do
  run_lint "$flag" "$TMP_ROOT/s9"
  if [[ "$RC" -eq 2 ]] && printf '%s' "$OUT" | grep -q 'nothing to bypass'; then
    pass "S10 $flag refused"
  else
    fail "S10 $flag was not refused: rc=$RC: $OUT"
  fi
done

# --------------------------------------------------------------------------
# S11 — a nonexistent path is a usage error, never a silent empty pass.
run_lint "$TMP_ROOT/does-not-exist.md"
if [[ "$RC" -eq 2 ]] && printf '%s' "$OUT" | grep -q 'no such path'; then
  pass "S11 nonexistent path is a usage error"
else
  fail "S11 expected rc 2 for a missing path, got rc=$RC: $OUT"
fi

# --------------------------------------------------------------------------
# S12 — an HTML entity is not a prose semicolon.
doc="$(new_doc s12 <<'MD'
Use &amp; and &lt; in the rendered output.
MD
)"
run_lint "$doc"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -q 'prose-semicolon:  *0'; then
  pass "S12 HTML entities are not prose semicolons"
else
  fail "S12 entity reported as a semicolon: rc=$RC: $OUT"
fi

# --------------------------------------------------------------------------
# S13 — a directory with no markdown reports and exits 0.
mkdir -p "$TMP_ROOT/s13"
printf 'nothing here\n' > "$TMP_ROOT/s13/readme.txt"
run_lint "$TMP_ROOT/s13"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -q 'nothing to report'; then
  pass "S13 empty markdown surface exits 0 with a report"
else
  fail "S13 expected the empty-surface report, got rc=$RC: $OUT"
fi

# --------------------------------------------------------------------------
# S14 — a hyphen-split registry term is reported.
printf '[{"term":"subagent","meaning":"x"}]\n' > "$TMP_ROOT/vocab.json"
doc="$(new_doc s14 <<'MD'
Dispatch the sub-agent and wait.
MD
)"
run_lint --vocabulary "$TMP_ROOT/vocab.json" "$doc"
if [[ "$RC" -eq 0 ]] \
  && printf '%s' "$OUT" | grep -q 'term-spelling:  *1'; then
  pass "S14 hyphen-split registry term reported"
else
  fail "S14 expected one term-spelling finding, got rc=$RC: $OUT"
fi

# --------------------------------------------------------------------------
# S15 — the correctly spelled term is NOT reported. Distinct registry terms
# that legitimately co-occur must never be counted as findings.
doc="$(new_doc s15 <<'MD'
Dispatch the subagent, then let the gate and its guard run.
MD
)"
run_lint --vocabulary "$TMP_ROOT/vocab.json" "$doc"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -q 'term-spelling:  *0'; then
  pass "S15 correct spelling and co-occurring terms are not findings"
else
  fail "S15 correct usage was reported: rc=$RC: $OUT"
fi

# --------------------------------------------------------------------------
if [[ "$FAILURES" -eq 0 ]]; then
  printf '\ntechnical-prose-lint-selftest: all cases passed.\n'
  exit 0
fi
printf '\ntechnical-prose-lint-selftest: %d case(s) failed.\n' "$FAILURES"
exit 1
