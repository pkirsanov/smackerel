#!/usr/bin/env bash
# bubbles/scripts/collected-test-count-guard.sh
#
# Refuse test evidence that proves nothing (IMP-036 SCOPE-3).
#
# WHY THIS EXISTS
# A downstream e2e suite executed ZERO tests for 15 days (2026-07-21 to
# 2026-08-05). Twelve spec commits were recorded in that window carrying
# passing-looking e2e evidence. Nothing caught it, because:
#
#   - the runner exited non-zero, which reads as ordinary test failure;
#   - the Execution Evidence Standard was satisfied, since raw output from a
#     broken runner is still raw output.
#
# Thirteen anti-fabrication gates and a >=10-line raw-output rule did not close
# this. The missing assertion was never "is there output" - it was "did any test
# actually run". A suite that collects nothing must never satisfy a test DoD
# item.
#
# WHAT IT CHECKS
# Only text INSIDE a fenced code block is considered. That is the discriminator
# that matters: captured runner output lives in a fence, prose does not. An
# earlier version scanned prose too and used a keyword window for context, which
# was circular - the sentence "No tests found in this area were affected"
# supplied its own context word and tripped the guard. Fence-scoping removes the
# whole class.
#
# Inside a fence, EXPLICIT zero-test signals across the common runners (jest,
# playwright, go, cargo, pytest, node --test, vitest, mocha) are a hard failure.
# That is deliberately narrow: it fires only when the captured output itself
# states that nothing ran.
#
# Evidence with no recognisable count at all is NOT failed. Runner formats vary
# too widely for absence to be proof, and a guard that fails on every
# unrecognised format gets bypassed rather than fixed.
#
# RATCHET, NOT A CLIFF
# A first scan across six repos found 25 genuine hits. Some are legitimate: a
# bug report's REPRODUCTION section is supposed to contain the failing output,
# and two of the hits sit in bugs named "missing-test-files-false-certification"
# and "disabled-fake-tests" - there the zero-test output IS the evidence. The
# guard cannot reliably tell a reproduction block from a passing claim, so the
# existing hits are frozen in <repo>/.specify/collected-test-count-guard.baseline
# and only NEW ones fail. The baseline may shrink, never grow. It is per-repo on
# purpose: each consuming repo owns and updates its own.
#
# Usage:
#   bash bubbles/scripts/collected-test-count-guard.sh <spec-dir-or-repo> [--verbose]
#   bash bubbles/scripts/collected-test-count-guard.sh <repo> --update-baseline
#
# Exit codes:
#   0 = no new zero-test evidence (or baseline updated)
#   1 = new evidence states that zero tests ran
#   2 = usage error, missing target, or a bypass-shaped flag

set -uo pipefail

usage() {
  cat <<'USAGE'
usage: collected-test-count-guard.sh <spec-dir-or-repo> [--verbose]

Fails when test evidence explicitly states that zero tests ran. Evidence with no
recognisable test count is reported, not failed.

There is no --skip, --force or --ignore flag. Evidence that proves nothing is
fixed by re-running the suite, never by suppressing the check.
USAGE
}

die_usage() {
  printf 'collected-test-count-guard: %s\n' "$1" >&2
  usage >&2
  exit 2
}

TARGET=""
VERBOSE="false"
UPDATE_BASELINE="false"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --verbose) VERBOSE="true" ;;
    --update-baseline) UPDATE_BASELINE="true" ;;
    -h|--help) usage; exit 0 ;;
    --skip*|--force*|--ignore*|--no-verify*|--bypass*|--allow*)
      die_usage "bypass-shaped flag '$1' is not supported and never will be" ;;
    -*) die_usage "unknown flag '$1'" ;;
    *) [[ -n "$TARGET" ]] && die_usage "unexpected extra argument '$1'"; TARGET="$1" ;;
  esac
  shift
done

[[ -n "$TARGET" ]] || die_usage "a spec directory or repo root is required (this tool has no default surface)"
[[ -d "$TARGET" ]] || die_usage "target does not exist: $TARGET"

# Explicit "nothing ran" signals. Each is a phrase a runner emits when it
# collected no tests. Anchored enough that ordinary prose does not match.
# Each alternative names a runner stating that IT COLLECTED NOTHING. The set is
# deliberately small, because two rounds against six real repos showed that
# generic patterns drown the signal:
#
#   - bare "0 total" matched jest's "Snapshots: 0 total" 354 times in one repo.
#     A suite with no snapshots is normal and is not a zero-test run.
#   - bare "0 passed" matched the tail of "10 passed"; bare "0 total" matched
#     "147 total". First run: 2,133 false hits.
#   - Go's per-package "ok <pkg> [no tests to run]" is routine for a package
#     with no test files and is reported as a PASS. It is excluded on purpose.
#
#   - "Tests:" without a leading word boundary matched "linkedTests: 0".
#   - "Tests: 0" without requiring "total" matched "Tests: 0 failed, all passed",
#     which is a PASSING run.
#
# What remains fires only when the runner says it collected nothing at all.
ZERO_SIGNALS='No tests found|no tests found|collected 0 items|Ran 0 tests|(^|[^a-zA-Z])Tests:[[:space:]]+0[[:space:]]+total|(^|[^a-zA-Z])Tests:[[:space:]]+0[[:space:]]+passed|(^|[^a-zA-Z])Test Suites:[[:space:]]+0[[:space:]]+total|(^|[^a-zA-Z])Test Suites:[[:space:]]+0[[:space:]]+passed|0 passed,[[:space:]]*0 failed,[[:space:]]*0 total|(^|[^0-9])0 (tests|specs) (ran|passed|executed)'

evidence_files="$(find "$TARGET" -type f \( -name 'report.md' -o -name 'scope.md' -o -name 'scopes.md' \) \
  -not -path '*/node_modules/*' -not -path '*/.git/*' 2>/dev/null | LC_ALL=C sort)"

scanned=0
violations=0
findings=""
observed=""
# The baseline is a PER-REPO ratchet, so it lives in the consuming repo and not
# beside this script. Two reasons, both load-bearing. Downstream this script
# installs under .github/bubbles/scripts/, which downstream-framework-write-guard.sh
# forbids the repo from editing, so a baseline there could never be updated by
# the only party entitled to update it. And a baseline beside the script in the
# framework repo would accumulate downstream spec paths, which breaks framework
# agnosticity and leaked an operator-linked location on the first attempt.
resolve_baseline_root() {
  local d
  d="$(cd "$TARGET" && pwd -P)"
  if command -v git >/dev/null 2>&1 && git -C "$d" rev-parse --show-toplevel >/dev/null 2>&1; then
    git -C "$d" rev-parse --show-toplevel
    return 0
  fi
  printf '%s' "$d"
}
BASELINE_FILE="${BUBBLES_ZERO_TEST_BASELINE_FILE:-$(resolve_baseline_root)/.specify/collected-test-count-guard.baseline}"
baseline=""
[[ -f "$BASELINE_FILE" ]] && baseline="$(grep -vE '^\s*(#|$)' "$BASELINE_FILE" 2>/dev/null)"

while IFS= read -r f; do
  [[ -n "$f" ]] || continue
  scanned=$((scanned + 1))
  while IFS= read -r hit; do
    [[ -n "$hit" ]] || continue
    lineno="${hit%%:*}"
    text="${hit#*:}"
    # Key on path + trimmed signal text, never the line number, so unrelated
    # edits above a block do not invalidate the whole baseline.
    trimmed="$(printf '%s' "$text" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')"
    key="${f#"$TARGET"/}|${trimmed}"
    observed="$observed$key"$'\n'
    printf '%s\n' "$baseline" | grep -qxF "$key" && continue
    violations=$((violations + 1))
    findings="$findings  ${f}:${lineno}
    ${trimmed}
"
  done < <(awk -v pat="$ZERO_SIGNALS" '
    /^[[:space:]]*```/ { infence = !infence; next }
    infence && $0 ~ pat { print NR ":" $0 }
  ' "$f" 2>/dev/null)
done <<EOF
$evidence_files
EOF

printf '[collected-test-count-guard] scanned %d evidence file(s)\n' "$scanned"

if [[ "$UPDATE_BASELINE" == "true" ]]; then
  mkdir -p "$(dirname "$BASELINE_FILE")" 2>/dev/null || true
  {
    printf '# collected-test-count-guard baseline (IMP-036 SCOPE-3)\n'
    printf '# Existing zero-test evidence blocks, keyed as <relpath>|<signal text>.\n'
    printf '# Some are legitimate: a bug reproduction section is SUPPOSED to show the\n'
    printf '# failing output. The guard cannot tell a reproduction from a passing claim,\n'
    printf '# so existing hits are frozen and only NEW ones fail.\n'
    printf '# This file may only SHRINK. Never add an entry to silence a failure.\n'
    printf '%s\n' "$(printf '%s' "$observed" | grep -v '^$' | LC_ALL=C sort -u)"
  } >"$BASELINE_FILE"
  printf 'collected-test-count-guard: baseline updated at %s\n' "$BASELINE_FILE"
  exit 0
fi

if [[ "$VERBOSE" == "true" ]]; then
  printf '[collected-test-count-guard] zero-signal patterns: %d\n' \
    "$(printf '%s' "$ZERO_SIGNALS" | tr '|' '\n' | grep -c . || true)"
fi

if [[ "$violations" -gt 0 ]]; then
  printf '\n[collected-test-count-guard] FAIL: %d evidence block(s) state that ZERO tests ran:\n\n' "$violations" >&2
  printf '%s' "$findings" >&2
  cat >&2 <<'REMEDY'

Evidence that reports zero collected tests proves nothing. It does not show the
feature works; it shows the runner did not execute.

Fix the suite so it collects tests, re-run it, and replace the evidence. Do not
re-word the evidence to hide the count.
REMEDY
  exit 1
fi

printf '[collected-test-count-guard] OK - no evidence claims a zero-test run\n'
exit 0
