#!/usr/bin/env bash
# effective-bundle-budget-selftest.sh — hermetic selftest for
# effective-bundle-budget.sh.
#
# Builds throwaway agent fixtures and asserts: no config is informational
# (exit 0); an over-budget agent fails ONLY in block mode; an over-budget agent
# is advisory (exit 0) without block; an under-budget agent passes; and a repo
# with no agents/ skips. Depends on effective-bundle-measure.sh (same dir).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUDGET="$SCRIPT_DIR/effective-bundle-budget.sh"

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

mk_repo() {
  mkdir -p "$1/agents"
  printf '# Tiny agent\nSome content here so the file has a non-trivial size.\n' > "$1/agents/tiny.agent.md"
  printf '%s' "$1"
}
set_cfg() {
  mkdir -p "$1/.github"
  printf '%s\n' "$2" > "$1/.github/bubbles-project.yaml"
}

# Case 1: no budget config → informational, exit 0
c1="$(mk_repo "$TMP_ROOT/c1")"
assert_exit 0 "Case 1: no config is informational" bash "$BUDGET" "$c1"

# Case 2: tiny budget + block → over budget, exit 1
c2="$(mk_repo "$TMP_ROOT/c2")"
set_cfg "$c2" "$(printf 'effectiveBundleMaxBytes: 1\neffectiveBundleBudget: block\n')"
assert_exit 1 "Case 2: over budget fails in block mode" bash "$BUDGET" "$c2"

# Case 3: tiny budget, no block → advisory, exit 0
c3="$(mk_repo "$TMP_ROOT/c3")"
set_cfg "$c3" "effectiveBundleMaxBytes: 1"
assert_exit 0 "Case 3: over budget is advisory by default" bash "$BUDGET" "$c3"

# Case 4: huge budget + block → within budget, exit 0
c4="$(mk_repo "$TMP_ROOT/c4")"
set_cfg "$c4" "$(printf 'effectiveBundleMaxBytes: 9999999\neffectiveBundleBudget: block\n')"
assert_exit 0 "Case 4: within budget passes" bash "$BUDGET" "$c4"

# Case 5: no agents/ directory → skip, exit 0
c5="$TMP_ROOT/c5"; mkdir -p "$c5"
assert_exit 0 "Case 5: no agents directory skips" bash "$BUDGET" "$c5"

echo
echo "effective-bundle-budget selftest: $pass passed, $fail failed"
[[ "$fail" -eq 0 ]]
