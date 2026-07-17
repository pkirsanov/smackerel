#!/usr/bin/env bash
# stale-deferral-lint-selftest.sh — hermetic selftest for stale-deferral-lint.sh.
#
# Cases (each builds a throwaway repo with its own VERSION + content files):
#   1. Clean fixture (no deferral)                         -> exit 0
#   2. Lapsed deferral: "deferred to v1.5", VERSION 2.0.0  -> exit 1
#   3. Future deferral: "deferred to v9.0", VERSION 2.0.0  -> exit 0 (legit)
#   4. Equal version:   "deferred to v2.0", VERSION 2.0.0  -> exit 1 (due now)
#   5. Patch-line cur:  "deferred to v2.0", VERSION 2.0.3  -> exit 1
#   6. "deferred until v1.0" variant, VERSION 2.0.0        -> exit 1
#   7. Excluded historical file (CHANGELOG.md) lapsed      -> exit 0
#   8. Excluded design doc (docs/v6-mcp-design.md) lapsed  -> exit 0
#   9. Missing VERSION file                                -> exit 1
#  10. Adversarial: lapsed deferral re-detected even when a
#      legit future deferral also exists in the same tree  -> exit 1
#  11. The lint's own selftest path is excluded             -> exit 0
#  12. Closed structured report evidence                    -> exit 0
#  13. Equivalent live report narrative                     -> exit 1
#  14. Structured report evidence missing metadata          -> exit 1
#  15. Structured report evidence with an unclosed fence     -> exit 1
#  16. Structured report evidence with a malformed close     -> exit 1
#  17. Shell-source fence in report evidence                 -> exit 1
#  18. Structured fenced text outside report.md              -> exit 1
#  19. Valid report evidence mixed with live narrative       -> exit 1
#
# Exit 0 = all cases pass. Exit 1 = at least one case behaved wrong.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINT="$SCRIPT_DIR/stale-deferral-lint.sh"

pass_count=0
fail_count=0
pass() { echo "PASS: $1"; pass_count=$((pass_count + 1)); }
fail() { echo "FAIL: $1" >&2; fail_count=$((fail_count + 1)); }

[[ -f "$LINT" ]] || { echo "FAIL: $LINT not found" >&2; exit 1; }

TMP="$(mktemp -d "${HOME}/.bubbles-selftest-deferral.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT INT TERM

# run_case <name> <fixture-dir> <expected-exit>
run_case() {
  local name="$1" dir="$2" want="$3"
  bash "$LINT" "$dir" >/dev/null 2>&1
  local got=$?
  if [[ "$got" -eq "$want" ]]; then
    pass "$name (exit $got)"
  else
    fail "$name expected exit $want, got $got"
  fi
}

# ── Case 1: clean ────────────────────────────────────────────────
c1="$TMP/c1"; mkdir -p "$c1/docs"
printf '2.0.0\n' > "$c1/VERSION"
printf 'All features shipped. Nothing pending.\n' > "$c1/docs/notes.md"
run_case "Case 1: clean tree" "$c1" 0

# ── Case 2: lapsed (< current) ───────────────────────────────────
c2="$TMP/c2"; mkdir -p "$c2/docs"
printf '2.0.0\n' > "$c2/VERSION"
printf 'Templated URIs are deferred to v1.5.\n' > "$c2/docs/mcp.md"
run_case "Case 2: lapsed deferred to v1.5" "$c2" 1

# ── Case 3: future (> current) is legitimate ─────────────────────
c3="$TMP/c3"; mkdir -p "$c3/docs"
printf '2.0.0\n' > "$c3/VERSION"
printf 'SSE streaming is deferred to v9.0.\n' > "$c3/docs/mcp.md"
run_case "Case 3: future deferred to v9.0 is allowed" "$c3" 0

# ── Case 4: equal to current is due now ──────────────────────────
c4="$TMP/c4"; mkdir -p "$c4/docs"
printf '2.0.0\n' > "$c4/VERSION"
printf 'This is deferred to v2.0.\n' > "$c4/docs/mcp.md"
run_case "Case 4: deferred to v2.0 at VERSION 2.0.0 is due" "$c4" 1

# ── Case 5: patch-level current still compares on MAJOR.MINOR ─────
c5="$TMP/c5"; mkdir -p "$c5/docs"
printf '2.0.3\n' > "$c5/VERSION"
printf 'This is deferred to v2.0.\n' > "$c5/docs/mcp.md"
run_case "Case 5: deferred to v2.0 at VERSION 2.0.3 is due" "$c5" 1

# ── Case 6: "deferred until" variant ─────────────────────────────
c6="$TMP/c6"; mkdir -p "$c6/docs"
printf '2.0.0\n' > "$c6/VERSION"
printf 'Held back, deferred until v1.0 originally.\n' > "$c6/docs/mcp.md"
run_case "Case 6: deferred until v1.0 variant" "$c6" 1

# ── Case 7: CHANGELOG.md is an excluded historical record ────────
c7="$TMP/c7"; mkdir -p "$c7"
printf '2.0.0\n' > "$c7/VERSION"
printf '## v1.0\n- HTTP deferred to v1.5.\n' > "$c7/CHANGELOG.md"
run_case "Case 7: CHANGELOG.md historical exclusion" "$c7" 0

# ── Case 8: docs/v6-mcp-design.md is an excluded design doc ──────
c8="$TMP/c8"; mkdir -p "$c8/docs"
printf '2.0.0\n' > "$c8/VERSION"
printf 'Transport: stdio; HTTP deferred to v1.1.\n' > "$c8/docs/v6-mcp-design.md"
run_case "Case 8: docs/v6-mcp-design.md exclusion" "$c8" 0

# ── Case 9: missing VERSION fails fast ───────────────────────────
c9="$TMP/c9"; mkdir -p "$c9/docs"
printf 'No version file here.\n' > "$c9/docs/mcp.md"
run_case "Case 9: missing VERSION fails" "$c9" 1

# ── Case 10 (ADVERSARIAL): a lapsed deferral must still be caught
# even when a legit future deferral also exists in the same tree.
# A naive "any deferral -> pass/fail" check would mis-handle this. ─
c10="$TMP/c10"; mkdir -p "$c10/docs"
printf '2.0.0\n' > "$c10/VERSION"
printf 'Future thing deferred to v9.0.\n' > "$c10/docs/future.md"
printf 'Old thing deferred to v1.0.\n' > "$c10/docs/old.md"
run_case "Case 10: lapsed caught alongside a legit future deferral" "$c10" 1

# ── Case 11: the lint's own selftest is excluded ─────────────────
# This file embeds lapsed-deferral strings as fixtures; a file at that exact
# path must be skipped so the live lint does not flag its own test data.
c11="$TMP/c11"; mkdir -p "$c11/bubbles/scripts"
printf '2.0.0\n' > "$c11/VERSION"
printf 'fixture: deferred to v1.0\n' > "$c11/bubbles/scripts/stale-deferral-lint-selftest.sh"
run_case "Case 11: own selftest path is excluded" "$c11" 0

# ── Case 12: closed, structured report evidence is historical ─────
c12="$TMP/c12"; mkdir -p "$c12"
printf '2.0.0\n' > "$c12/VERSION"
printf '%s\n' \
  '## Historical execution evidence' \
  '**Phase:** regression' \
  '**Command:** `bash bubbles/scripts/example-selftest.sh`' \
  '**Exit Code:** 0' \
  '**Claim Source:** executed' \
  '```text' \
  'Historical verdict: transport deferred to v1.0.' \
  '```' > "$c12/report.md"
run_case "Case 12: closed structured report evidence is allowed" "$c12" 0

# ── Case 13: equivalent live narrative remains live policy ────────
c13="$TMP/c13"; mkdir -p "$c13"
printf '2.0.0\n' > "$c13/VERSION"
printf '%s\n' \
  '## Current status' \
  'Historical verdict: transport deferred to v1.0.' > "$c13/report.md"
run_case "Case 13: equivalent live report narrative fails" "$c13" 1

# ── Case 14: incomplete evidence metadata fails closed ─────────────
c14="$TMP/c14"; mkdir -p "$c14"
printf '2.0.0\n' > "$c14/VERSION"
printf '%s\n' \
  '## Incomplete execution evidence' \
  '**Phase:** regression' \
  '**Command:** `bash bubbles/scripts/example-selftest.sh`' \
  '**Claim Source:** executed' \
  '```text' \
  'Historical verdict: transport deferred to v1.0.' \
  '```' > "$c14/report.md"
run_case "Case 14: incomplete evidence metadata fails" "$c14" 1

# ── Case 15: an unclosed candidate is scanned at EOF ───────────────
c15="$TMP/c15"; mkdir -p "$c15"
printf '2.0.0\n' > "$c15/VERSION"
printf '%s\n' \
  '## Unclosed execution evidence' \
  '**Phase:** regression' \
  '**Command:** `bash bubbles/scripts/example-selftest.sh`' \
  '**Exit Code:** 0' \
  '**Claim Source:** executed' \
  '```text' \
  'Historical verdict: transport deferred to v1.0.' > "$c15/report.md"
run_case "Case 15: unclosed text fence fails" "$c15" 1

# ── Case 16: a malformed or mismatched close is not a close ────────
c16="$TMP/c16"; mkdir -p "$c16"
printf '2.0.0\n' > "$c16/VERSION"
printf '%s\n' \
  '## Malformed execution evidence' \
  '**Phase:** regression' \
  '**Command:** `bash bubbles/scripts/example-selftest.sh`' \
  '**Exit Code:** 0' \
  '**Claim Source:** executed' \
  '```text' \
  'Historical verdict: transport deferred to v1.0.' \
  '````' \
  '```' > "$c16/report.md"
run_case "Case 16: malformed or mismatched fence close fails" "$c16" 1

# ── Case 17: executable shell/source fences remain scanner input ───
c17="$TMP/c17"; mkdir -p "$c17"
printf '2.0.0\n' > "$c17/VERSION"
printf '%s\n' \
  '## Source evidence' \
  '**Phase:** regression' \
  '**Command:** `bash bubbles/scripts/example-selftest.sh`' \
  '**Exit Code:** 0' \
  '**Claim Source:** executed' \
  '```bash' \
  'printf "%s\\n" "transport deferred to v1.0"' \
  '```' > "$c17/report.md"
run_case "Case 17: shell-source fence fails" "$c17" 1

# ── Case 18: the exemption never applies outside report.md ─────────
c18="$TMP/c18"; mkdir -p "$c18/docs"
printf '2.0.0\n' > "$c18/VERSION"
printf '%s\n' \
  '## Historical execution evidence' \
  '**Phase:** regression' \
  '**Command:** `bash bubbles/scripts/example-selftest.sh`' \
  '**Exit Code:** 0' \
  '**Claim Source:** executed' \
  '```text' \
  'Historical verdict: transport deferred to v1.0.' \
  '```' > "$c18/docs/notes.md"
run_case "Case 18: fenced text outside report.md fails" "$c18" 1

# ── Case 19: valid evidence cannot hide adjacent live narrative ────
c19="$TMP/c19"; mkdir -p "$c19"
printf '2.0.0\n' > "$c19/VERSION"
printf '%s\n' \
  '## Historical execution evidence' \
  '**Phase:** regression' \
  '**Commands:** `bash bubbles/scripts/example-selftest.sh`' \
  '**Exit Codes:** 0' \
  '**Claim Source:** executed' \
  '```text' \
  'Historical verdict: transport deferred to v1.0.' \
  '```' \
  '' \
  '## Current policy' \
  'Live transport remains deferred until v1.1.' > "$c19/report.md"
run_case "Case 19: valid evidence mixed with live narrative fails" "$c19" 1

echo
echo "stale-deferral-lint-selftest: $pass_count pass, $fail_count fail"
[[ "$fail_count" -eq 0 ]] || exit 1
exit 0
