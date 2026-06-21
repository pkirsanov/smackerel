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

echo
echo "artifact-lint certifying-window selftest: $passes/$assertions assertions passed"
