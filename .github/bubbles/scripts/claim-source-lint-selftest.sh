#!/usr/bin/env bash
# claim-source-lint-selftest.sh — hermetic selftest for claim-source-lint.sh.
#
# Builds throwaway report.md fixtures and asserts: a proper executed block
# passes; an execution block missing the tag fails ONLY in block mode (advisory
# by default); an invalid tag value fails in block mode; and a not-run block
# (no Exit Code) passes. No network, no dependency on the live tree.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINT="$SCRIPT_DIR/claim-source-lint.sh"

pass=0
fail=0
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT

assert_exit() {
  local expected="$1" label="$2"; shift 2
  local actual=0
  "$@" >/dev/null 2>&1 || actual=$?
  if [[ "$actual" -eq "$expected" ]]; then
    echo "PASS: $label (exit $actual)"
    pass=$((pass + 1))
  else
    echo "FAIL: $label (expected exit $expected, got $actual)"
    fail=$((fail + 1))
  fi
}

enable_block() {
  mkdir -p "$1/.github"
  printf 'claimSourceProvenanceGuard: block\n' > "$1/.github/bubbles-project.yaml"
}

# Case 1: a proper executed block passes (advisory, no config)
c1="$TMP_ROOT/c1"; mkdir -p "$c1/specs/f"
printf '**Phase:** test\n**Command:** [test]\n**Exit Code:** 0\n**Claim Source:** executed\nok PASS line\n' > "$c1/specs/f/report.md"
assert_exit 0 "Case 1: proper executed block passes" bash "$LINT" "$c1"

# Case 2: execution block missing the tag fails in BLOCK mode
c2="$TMP_ROOT/c2"; mkdir -p "$c2/specs/f"; enable_block "$c2"
printf '**Phase:** test\n**Command:** [test]\n**Exit Code:** 0\nok PASS line\n' > "$c2/specs/f/report.md"
assert_exit 1 "Case 2: missing tag fails in block mode" bash "$LINT" "$c2"

# Case 3: same missing tag is ADVISORY (exit 0) with no config
c3="$TMP_ROOT/c3"; mkdir -p "$c3/specs/f"
printf '**Exit Code:** 0\nok\n' > "$c3/specs/f/report.md"
assert_exit 0 "Case 3: missing tag is advisory by default" bash "$LINT" "$c3"

# Case 4: an invalid Claim Source value fails in BLOCK mode
c4="$TMP_ROOT/c4"; mkdir -p "$c4/specs/f"; enable_block "$c4"
printf '**Command:** [test]\n**Exit Code:** 0\n**Claim Source:** guessed\nok\n' > "$c4/specs/f/report.md"
assert_exit 1 "Case 4: invalid tag value fails in block mode" bash "$LINT" "$c4"

# Case 5: a not-run block (no Exit Code) passes even in BLOCK mode
c5="$TMP_ROOT/c5"; mkdir -p "$c5/specs/f"; enable_block "$c5"
printf '**Phase:** test\n**Claim Source:** not-run\n**Reason:** timed out\n' > "$c5/specs/f/report.md"
assert_exit 0 "Case 5: not-run block passes in block mode" bash "$LINT" "$c5"

# Case 6: a proper interpreted block passes in BLOCK mode
c6="$TMP_ROOT/c6"; mkdir -p "$c6/specs/f"; enable_block "$c6"
printf '**Command:** [test]\n**Exit Code:** 0\n**Claim Source:** interpreted\n**Interpretation:** x\nok\n' > "$c6/specs/f/report.md"
assert_exit 0 "Case 6: proper interpreted block passes in block mode" bash "$LINT" "$c6"

echo
echo "claim-source-lint selftest: $pass passed, $fail failed"
[[ "$fail" -eq 0 ]]
