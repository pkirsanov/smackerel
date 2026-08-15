#!/usr/bin/env bash
# artifact-lint-selftest.sh — hermetic selftest for the Check 3 (evidence
# legitimacy) certifying-window boundary marker in artifact-lint.sh.
#
# The <!-- bubbles:certifying-window-begin --> marker (report.md only, opt-in,
# at most one per file) splits report.md into a prior-window history region
# (every code block BEFORE the marker) and the current certifying window (every
# code block AFTER it). Pre-marker blocks are exempted from the done-strict
# >=3-line / >=2-signal heuristic (treated like the evidence-legitimacy-skip
# region); post-marker blocks stay fully enforced.
#
# Adversarial assertions:
#   T1. Marker present -> a compact PRE-marker block is EXEMPT (skip info line;
#       no Check-3 failure for it) AND a signal-rich POST-marker block passes.
#   T2. Marker present -> a weak POST-marker block is ENFORCED (Check-3 fails)
#       while the compact PRE-marker block stays exempt.
#   T3. Two markers     -> fail loud ("Multiple ... markers (2)").
#   T4. NO marker       -> a weak block is STILL ENFORCED (no silent fleet-wide
#       disable — the integrity guarantee that the exemption is opt-in per file).
#   T5-T8. Current-window duplicate evidence is rejected without treating
#       distinct dimensions or prior-window history as duplicates.
#   T9. Canonical v3    -> nested certification.scopeProgress plus current
#       scopeLayout/statusDiscipline fields produce no deprecation warning.
#   T10. Legacy v3      -> top-level scopeProgress still produces a warning.
#
# Check-3 only runs at state.json status == "done"; every fixture sets that.
# The overall lint exit code is non-zero (minimal fixtures omit spec/design/
# scopes), so assertions target Check-3's specific stdout lines, NOT exit code.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINT="$SCRIPT_DIR/artifact-lint.sh"

[[ -f "$LINT" ]] || {
  echo "FAIL: artifact-lint.sh not found next to selftest ($LINT)" >&2
  exit 1
}

TMP="$(mktemp -d "${HOME}/.bubbles-selftest-artifact-lint.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

assertions=0
passes=0

# make_fixture <name> — create specs/<name>/ with a done state.json. The caller
# writes report.md afterwards. Echoes the absolute feature directory path.
make_fixture() {
  local name="$1"
  local dir="$TMP/specs/$name"
  rm -rf "$dir"
  mkdir -p "$dir"
  cat > "$dir/state.json" <<'STATE'
{
  "status": "done",
  "schemaVersion": 3,
  "certification": { "status": "done" }
}
STATE
  printf '%s\n' "$dir"
}

run_lint() {
  bash "$LINT" "$1" 2>&1 || true
}

expect_in() {
  local desc="$1" haystack="$2" needle="$3"
  assertions=$((assertions + 1))
  if printf '%s' "$haystack" | grep -qF -- "$needle"; then
    echo "PASS: $desc"
    passes=$((passes + 1))
  else
    echo "FAIL: $desc" >&2
    echo "  expected to find: $needle" >&2
    echo "  --- lint output ---" >&2
    printf '%s\n' "$haystack" >&2
    exit 1
  fi
}

expect_not_in() {
  local desc="$1" haystack="$2" needle="$3"
  assertions=$((assertions + 1))
  if printf '%s' "$haystack" | grep -qF -- "$needle"; then
    echo "FAIL: $desc" >&2
    echo "  did NOT expect to find: $needle" >&2
    echo "  --- lint output ---" >&2
    printf '%s\n' "$haystack" >&2
    exit 1
  else
    echo "PASS: $desc"
    passes=$((passes + 1))
  fi
}

# ── T1: marker present → pre-marker compact block EXEMPT, post-marker rich passes
d="$(make_fixture cw-pre-exempt)"
cat > "$d/report.md" <<'RPT'
# Report

Pre-window historical evidence:
```
(no output — historical container state, not reproducible)
```

<!-- bubbles:certifying-window-begin -->

Post-window fresh evidence:
```
$ cargo test
running 12 tests
test result: ok. 12 passed; 0 failed; finished in 1.23s
```
RPT
out="$(run_lint "$d")"
expect_in "T1 pre-marker compact block is exempted (prior-window skip)" \
  "$out" "Skipped 1 evidence blocks before <!-- bubbles:certifying-window-begin -->"
expect_not_in "T1 pre-marker compact block is NOT flagged by Check-3" \
  "$out" "Pre-window historical evidence"
expect_in "T1 signal-rich post-marker block passes Check-3" \
  "$out" "contain legitimate terminal output"

# ── T2: marker present → weak POST-marker block ENFORCED, PRE-marker stays exempt
d="$(make_fixture cw-post-enforced)"
cat > "$d/report.md" <<'RPT'
# Report

Pre-window historical evidence:
```
historical-only
```

<!-- bubbles:certifying-window-begin -->

Post-window weak evidence:
```
TODO
```
RPT
out="$(run_lint "$d")"
expect_in "T2 weak post-marker block is enforced (too-short failure)" \
  "$out" "Evidence block too short"
expect_in "T2 the enforced failure names the post-marker block" \
  "$out" "Post-window weak evidence"
expect_not_in "T2 the pre-marker block is NOT the one flagged" \
  "$out" "too short (1 lines): Pre-window historical evidence"
expect_in "T2 pre-marker block still counted as prior-window skip" \
  "$out" "Skipped 1 evidence blocks before <!-- bubbles:certifying-window-begin -->"

# ── T3: two markers → fail loud (ambiguous window start)
d="$(make_fixture cw-duplicate)"
cat > "$d/report.md" <<'RPT'
# Report

<!-- bubbles:certifying-window-begin -->

Block A:
```
$ echo hi
hi ok
finished in 0.1s
```

<!-- bubbles:certifying-window-begin -->

Block B:
```
$ echo bye
bye ok
finished in 0.1s
```
RPT
out="$(run_lint "$d")"
expect_in "T3 duplicate certifying-window markers fail loud" \
  "$out" "Multiple <!-- bubbles:certifying-window-begin --> markers (2)"

# ── T4: NO marker → weak block STILL ENFORCED (integrity: opt-in per file)
d="$(make_fixture cw-no-marker)"
cat > "$d/report.md" <<'RPT'
# Report

Unmarked weak evidence:
```
TODO
```
RPT
out="$(run_lint "$d")"
expect_in "T4 marker-less report still enforces Check-3 (anti-leak)" \
  "$out" "Evidence block too short"
expect_in "T4 the enforced failure names the unmarked block" \
  "$out" "Unmarked weak evidence"
expect_not_in "T4 no prior-window skip happens without a marker" \
  "$out" "Skipped 1 evidence blocks before <!-- bubbles:certifying-window-begin -->"

# ── T5 (BFW-04): marker present → two IDENTICAL post-marker blocks → the 2nd is a
#    redundant re-verification of an already-verified identical result → flagged.
d="$(make_fixture cw-dup-identical)"
cat > "$d/report.md" <<'RPT'
# Report

<!-- bubbles:certifying-window-begin -->

TP-01 first verification:
```
$ cargo test
running 5 tests
test result: ok. 5 passed; 0 failed; finished in 0.42s
```

TP-01 re-verified again (byte-identical result, different prose header):
```
$ cargo test
running 5 tests
test result: ok. 5 passed; 0 failed; finished in 0.42s
```
RPT
out="$(run_lint "$d")"
expect_in "T5 identical re-pasted evidence in the current window is flagged (BFW-04)" \
  "$out" "Redundant identical evidence block in the current certifying window"

# ── T6 (BFW-04): marker present → two DISTINCT post-marker blocks → NOT flagged.
#    Proves different-dimension re-runs (different output) are never touched.
d="$(make_fixture cw-dup-distinct)"
cat > "$d/report.md" <<'RPT'
# Report

<!-- bubbles:certifying-window-begin -->

Unit test evidence:
```
$ cargo test
running 5 tests
test result: ok. 5 passed; 0 failed; finished in 0.42s
```

E2E test evidence (different dimension, different output):
```
$ npm run test:e2e
running 3 tests
3 passed, 0 failed in 2.10s
```
RPT
out="$(run_lint "$d")"
expect_not_in "T6 distinct evidence blocks are NOT flagged as duplicates" \
  "$out" "Redundant identical evidence block"
expect_in "T6 distinct evidence blocks both pass Check-3" \
  "$out" "contain legitimate terminal output"

# ── T7 (BFW-04): NO marker → identical blocks NOT flagged (duplicate detection is
#    opt-in per certifying-window marker, so grandfathered reports are untouched).
d="$(make_fixture cw-dup-nomarker)"
cat > "$d/report.md" <<'RPT'
# Report

First:
```
$ cargo test
running 5 tests
test result: ok. 5 passed; 0 failed; finished in 0.42s
```

Second (identical, but no window marker):
```
$ cargo test
running 5 tests
test result: ok. 5 passed; 0 failed; finished in 0.42s
```
RPT
out="$(run_lint "$d")"
expect_not_in "T7 identical blocks in a marker-less report are NOT flagged (opt-in)" \
  "$out" "Redundant identical evidence block"

# ── T8 (BFW-04): identical block PRE- and POST-marker → post-marker NOT flagged
#    (prior-window history is skipped, so it is not a prior in-window fingerprint).
d="$(make_fixture cw-dup-prewindow)"
cat > "$d/report.md" <<'RPT'
# Report

Prior-window evidence:
```
$ cargo test
running 5 tests
test result: ok. 5 passed; 0 failed; finished in 0.42s
```

<!-- bubbles:certifying-window-begin -->

Current-window evidence (same command, first time in this window):
```
$ cargo test
running 5 tests
test result: ok. 5 passed; 0 failed; finished in 0.42s
```
RPT
out="$(run_lint "$d")"
expect_not_in "T8 a post-marker block identical to a PRE-marker block is NOT flagged" \
  "$out" "Redundant identical evidence block"
expect_in "T8 the pre-marker identical block is a prior-window skip" \
  "$out" "Skipped 1 evidence blocks before"

# ── T9: canonical v3 fields → no stale v2 deprecation warnings
d="$(make_fixture canonical-v3-fields)"
cat > "$d/state.json" <<'STATE'
{
  "version": 3,
  "status": "not_started",
  "workflowMode": "full-delivery",
  "execution": {
    "completedPhaseClaims": []
  },
  "certification": {
    "status": "not_started",
    "completedScopes": [],
    "certifiedCompletedPhases": [],
    "scopeProgress": [],
    "lockdownState": { "active": false, "lockedScenarioIds": [] }
  },
  "policySnapshot": {},
  "scopeLayout": "single-file",
  "statusDiscipline": {}
}
STATE
out="$(run_lint "$d")"
expect_not_in "T9 nested certification.scopeProgress is current v3 state" \
  "$out" "uses deprecated field 'scopeProgress'"
expect_not_in "T9 scopeLayout is current v3 state" \
  "$out" "uses deprecated field 'scopeLayout'"
expect_not_in "T9 statusDiscipline is current v3 state" \
  "$out" "uses deprecated field 'statusDiscipline'"

# ── T10: legacy top-level scopeProgress remains detected
d="$(make_fixture legacy-top-level-scope-progress)"
cat > "$d/state.json" <<'STATE'
{
  "version": 3,
  "status": "not_started",
  "workflowMode": "full-delivery",
  "execution": {
    "completedPhaseClaims": []
  },
  "certification": {
    "status": "not_started",
    "completedScopes": [],
    "certifiedCompletedPhases": [],
    "scopeProgress": [],
    "lockdownState": { "active": false, "lockedScenarioIds": [] }
  },
  "policySnapshot": {},
  "scopeProgress": []
}
STATE
out="$(run_lint "$d")"
expect_in "T10 top-level scopeProgress remains deprecated" \
  "$out" "uses deprecated field 'scopeProgress'"

# --- T11: the array extractors must not truncate ------------------------------
# extract_nested_array_block was `grep -A60 | awk '... /\]/ {exit}'`: it dropped
# everything past 60 lines AND exited at the first nested `]`. A live packet whose
# completedPhaseClaims spanned 96 lines had its trailing `implement` claims fall
# outside the window, and the linter reported `implement` MISSING while the
# eleven phases inside the window passed. Under-reading here does not soften a
# check — G022's message is "FABRICATION", so a truncated read accuses an honest
# record of forging its history.
d="$(make_fixture t11-long-claims)"
python3 - "$d/state.json" <<'PY'
import json, sys
# Ten leading claims push the array well past the old 60-line window, and each
# carries a NESTED array so the old first-`]` exit would also have fired early.
claims = [{"phase": f"filler{i}", "agent": "bubbles.test", "scopes": [1, 2],
           "claim": "padding entry to push the tail past the old window"}
          for i in range(10)]
claims.append({"phase": "implement", "agent": "bubbles.implement", "scopes": [1],
               "claim": "the tail entry the old extractor could not see"})
json.dump({
    "status": "done",
    "schemaVersion": 3,
    "workflowMode": "full-delivery",
    "execution": {"completedPhaseClaims": claims},
    "certification": {"status": "done", "certifiedCompletedPhases": []},
}, open(sys.argv[1], "w"), indent=2)
PY
claims_lines="$(python3 -c "
import sys
raw = open(sys.argv[1]).read()
start = raw.index('\"completedPhaseClaims\"')
print(raw[start:].count(chr(10)))
" "$d/state.json")"
assertions=$((assertions + 1))
if [[ "$claims_lines" -gt 60 ]]; then
  echo "PASS: T11 fixture array spans $claims_lines lines, past the old 60-line window"
  passes=$((passes + 1))
else
  echo "FAIL: T11 fixture array spans only $claims_lines lines — it would not have tripped the old truncation, so this case proves nothing" >&2
  exit 1
fi
out="$(run_lint "$d")"
# POSITIVE assertion, deliberately: "not reported missing" alone would also hold
# if the specialist check never ran at all, so this case would pass while proving
# nothing. Requiring the FOUND line proves the check ran AND read past the window.
expect_in "T11 the trailing claim past the old window is FOUND" \
  "$out" "Required specialist phase 'implement' found"
expect_not_in "T11 the trailing claim is not accused of fabrication" \
  "$out" "Required specialist phase 'implement' missing"

# ── T12: a declared non-transcript fence (gherkin) is EXEMPT from the signal
# heuristic. A specification cannot carry an exit code, so demanding one forces
# the author to invent output no command produced.
d="$(make_fixture nontranscript-gherkin-exempt)"
cat > "$d/report.md" <<'RPT'
# Report

The scenario this fix preserves:

```gherkin
  Scenario: An empty attention tier with no recorded exclusions is refused
    Given a committed payload whose attention tier is empty
     When the publication gate runs
     Then publication is refused by name
```
RPT
out="$(run_lint "$d")"
expect_not_in "T12 a gherkin block is not accused of lacking terminal signals" \
  "$out" "lacks terminal output signals"
expect_in "T12 the exemption is reported rather than silent" \
  "$out" "non-transcript block"

# ── T13: ADVERSARIAL CONTROL for T12. The SAME content in a BARE fence is still
# enforced. Without this, T12 would also pass if the exemption had been written
# as a blanket disable of the whole check, proving nothing about it being
# language-gated.
d="$(make_fixture nontranscript-bare-still-enforced)"
cat > "$d/report.md" <<'RPT'
# Report

The same text, but claimed as output:

```
  Scenario: An empty attention tier with no recorded exclusions is refused
    Given a committed payload whose attention tier is empty
     When the publication gate runs
     Then publication is refused by name
```
RPT
out="$(run_lint "$d")"
expect_in "T13 the identical content in a BARE fence is still enforced" \
  "$out" "lacks terminal output signals"

# ── T14: an .mjs path counts as a file-path signal. In a build-free ESM repo a
# real executed command line is the ONLY path form available, and it previously
# scored nothing because the alternation carried js but not mjs.
d="$(make_fixture esm-path-signal)"
cat > "$d/report.md" <<'RPT'
# Report

Executed:

```
$ node scripts/selftest.mjs
(suite emitted no diagnostics)
(run complete)
```
RPT
out="$(run_lint "$d")"
expect_not_in "T14 an .mjs command line supplies a second signal" \
  "$out" "lacks terminal output signals"

# ── T15: ADVERSARIAL CONTROL for T14. An unknown extension still scores only the
# shell-prompt signal and is still refused, proving T14 passes because mjs was
# recognised and not because the path check was loosened to match anything.
d="$(make_fixture esm-path-signal-control)"
cat > "$d/report.md" <<'RPT'
# Report

Executed:

```
$ node scripts/selftest.zzq
(suite emitted no diagnostics)
(run complete)
```
RPT
out="$(run_lint "$d")"
expect_in "T15 an unrecognised extension is still refused (1/2 signals)" \
  "$out" "lacks terminal output signals"

# ── T16: a ROOT-LEVEL filename supplies the path signal. A flat repository (every
# file at the root, no directory prefix anywhere) could not emit this signal while
# the pattern demanded a `dir/` prefix, so real output referencing a real file was
# refused for the shape of the repository rather than the quality of the evidence.
d="$(make_fixture flat-repo-path-signal)"
cat > "$d/report.md" <<'RPT'
# Report

Executed:

```
$ node -e "..."
market-brief.payload.json => attention: 3 item(s)
(comparison complete)
```
RPT
out="$(run_lint "$d")"
expect_not_in "T16 a root-level filename supplies a second signal" \
  "$out" "lacks terminal output signals"

# ── T17: ADVERSARIAL CONTROL for T16. Prose with no extension-bearing filename is
# still refused, proving T16 passes because a real file was named and not because
# the path check was loosened into matching ordinary words.
d="$(make_fixture flat-repo-path-signal-control)"
cat > "$d/report.md" <<'RPT'
# Report

Executed:

```
$ node -e "..."
the comparison completed and the tier looked correct
(comparison complete)
```
RPT
out="$(run_lint "$d")"
expect_in "T17 prose naming no file is still refused (1/2 signals)" \
  "$out" "lacks terminal output signals"

echo
echo "artifact-lint selftest: $passes/$assertions assertions passed"
