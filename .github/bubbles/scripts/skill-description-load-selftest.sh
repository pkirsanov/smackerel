#!/usr/bin/env bash
# skill-description-load-selftest.sh — hermetic selftest for
# skill-description-load.sh (IMP-021 SCOPE-5). Builds throwaway skills/ +
# INVENTORY.md fixtures, then proves the deliverables:
#
#   (b) the aggregate report RUNS and SUMS the auto-discovery description bytes
#       from a fixture with known byte counts (exit 0, correct auto total).
#   (c) an ADVERSARIAL case — a skill row missing its Invocation class — is
#       FLAGGED by the report (exit 1, names the skill).
#   plus: a missing DescBytes cell is flagged; an un-upgraded (no Invocation/
#       DescBytes column) inventory is flagged; column-indexed parsing ignores
#       class-like text in the Notes column; usage errors exit 2.
#   (a) LIVE, source-only: the REAL skills/INVENTORY.md carries an Invocation +
#       DescBytes column for every skill and the live report exits 0. Skipped
#       gracefully when the source inventory is absent (downstream install).
#
# Exit 0 = all cases pass. Exit 1 = a case failed.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPORT="$SCRIPT_DIR/skill-description-load.sh"

[[ -x "$REPORT" ]] || {
  echo "FAIL: $REPORT not executable" >&2
  exit 1
}

TMP="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-selftest-skill-descload.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

reset_fixture() {
  rm -rf "$TMP/skills"
  mkdir -p "$TMP/skills"
}

# make_skill <name> <description> — a skills/<name>/ dir whose SKILL.md carries
# the given single-line description. The description value's byte length is what
# the report sums.
make_skill() {
  mkdir -p "$TMP/skills/$1"
  printf -- '---\nname: %s\ndescription: %s\n---\n# %s\n' "$1" "$2" "$1" >"$TMP/skills/$1/SKILL.md"
}

# write_inventory_row helpers build a 6-column table:
#   | Skill | LOC | Invocation | DescBytes | Status | Notes |
inv_open() {
  {
    echo "# Fixture Skills Inventory"
    echo
    echo "| Status | Meaning |"
    echo "|---|---|"
    echo "| **KEEP** | decision-matrix row — must be ignored by the report parser. |"
    echo
    echo "| Skill | LOC | Invocation | DescBytes | Status | Notes |"
    echo "|---|---:|---|---:|---|---|"
  } >"$TMP/skills/INVENTORY.md"
}
inv_row() { # <name> <loc> <invocation> <descbytes> <notes>
  echo "| \`$1\` | $2 | $3 | $4 | KEEP | $5 |" >>"$TMP/skills/INVENTORY.md"
}

# assert_exit <desc> <want-code> <cmd...>
assert_exit() {
  local desc="$1" want="$2"
  shift 2
  local rc=0
  "$@" >/dev/null 2>&1 || rc=$?
  if [[ "$rc" -eq "$want" ]]; then
    echo "PASS: $desc (exit $rc)"
  else
    echo "FAIL: $desc (expected exit $want, got $rc)" >&2
    "$@" || true
    exit 1
  fi
}

# assert_stdout_contains <desc> <needle> <cmd...>
assert_stdout_contains() {
  local desc="$1" needle="$2"
  shift 2
  local out
  out="$("$@" 2>/dev/null || true)"
  if grep -qF "$needle" <<<"$out"; then
    echo "PASS: $desc (stdout has '$needle')"
  else
    echo "FAIL: $desc (stdout lacked '$needle')" >&2
    echo "$out" >&2
    exit 1
  fi
}

# assert_stderr_names <desc> <needle> <cmd...>
assert_stderr_names() {
  local desc="$1" needle="$2"
  shift 2
  local out
  out="$("$@" 2>&1 || true)"
  if grep -qF "$needle" <<<"$out"; then
    echo "PASS: $desc (mentions '$needle')"
  else
    echo "FAIL: $desc (stderr did not mention '$needle')" >&2
    echo "$out" >&2
    exit 1
  fi
}

# (b) well-formed fixture: auto=alpha(4)+beta(6)=10, explicit=gamma(2). ------
# The gamma Notes cell deliberately contains the literal token
# 'auto-discovery-required' to prove the parser reads the class from the
# Invocation COLUMN, not from anywhere in the row.
reset_fixture
make_skill bubbles-alpha "AAAA"
make_skill bubbles-beta "BBBBBB"
make_skill bubbles-gamma "CC"
inv_open
inv_row bubbles-alpha 50 "auto-discovery-required" 4 "discovery skill."
inv_row bubbles-beta 40 "auto-discovery-required" 6 "discovery skill."
inv_row bubbles-gamma 30 "explicit-invocation-sufficient" 2 "reached via a specific agent; not auto-discovery-required by default."
assert_exit "well-formed inventory report exits 0" 0 bash "$REPORT" --repo-root "$TMP"
assert_stdout_contains "sums auto-discovery bytes (4+6=10)" "auto-discovery-required:          2 skill(s), 10 bytes" bash "$REPORT" --repo-root "$TMP"
assert_stdout_contains "sums explicit bytes (2)" "explicit-invocation-sufficient:   1 skill(s), 2 bytes" bash "$REPORT" --repo-root "$TMP"
assert_stdout_contains "sums all descriptions (12)" "all skill descriptions:           12 bytes" bash "$REPORT" --repo-root "$TMP"

# (c) ADVERSARIAL: a skill row with an EMPTY Invocation class → exit 1, named --
reset_fixture
make_skill bubbles-alpha "AAAA"
make_skill bubbles-beta "BBBBBB"
inv_open
inv_row bubbles-alpha 50 "auto-discovery-required" 4 "discovery skill."
inv_row bubbles-beta 40 "" 6 "MISSING class cell."
assert_exit "missing Invocation class fails" 1 bash "$REPORT" --repo-root "$TMP"
assert_stderr_names "names the class-less skill" "bubbles-beta" bash "$REPORT" --repo-root "$TMP"

# adversarial: a skill row with a NON-numeric DescBytes → exit 1, named --------
reset_fixture
make_skill bubbles-alpha "AAAA"
inv_open
inv_row bubbles-alpha 50 "auto-discovery-required" "n/a" "bad DescBytes cell."
assert_exit "non-numeric DescBytes fails" 1 bash "$REPORT" --repo-root "$TMP"
assert_stderr_names "names the byte-less skill" "bubbles-alpha" bash "$REPORT" --repo-root "$TMP"

# adversarial: an un-upgraded inventory (no Invocation/DescBytes columns) fails -
reset_fixture
make_skill bubbles-alpha "AAAA"
{
  echo "# Fixture Skills Inventory"
  echo
  echo "| Skill | LOC | Status | Notes |"
  echo "|---|---:|---|---|"
  echo "| \`bubbles-alpha\` | 50 | KEEP | old 4-column row. |"
} >"$TMP/skills/INVENTORY.md"
assert_exit "un-upgraded inventory (no new columns) fails" 1 bash "$REPORT" --repo-root "$TMP"

# usage errors → exit 2 -------------------------------------------------------
reset_fixture
make_skill bubbles-alpha "AAAA" # skills/ exists but no INVENTORY.md written
assert_exit "missing INVENTORY.md is a usage error" 2 bash "$REPORT" --repo-root "$TMP"
assert_exit "-h is a usage exit" 2 bash "$REPORT" -h

# (a) LIVE (source-only): the REAL skills/INVENTORY.md carries both columns and
#     a class for every skill; the live report exits 0. SKIP when absent.
if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  REAL_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
else
  REAL_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi
REAL_INV="$REAL_ROOT/skills/INVENTORY.md"
if [[ -f "$REAL_INV" ]]; then
  if grep -qE '^\|.*Skill.*Invocation.*DescBytes' "$REAL_INV"; then
    echo "PASS: real INVENTORY.md header carries Invocation + DescBytes columns"
  else
    echo "FAIL: real INVENTORY.md header lacks Invocation/DescBytes columns" >&2
    exit 1
  fi
  assert_exit "live report on real inventory exits 0 (every skill has a class)" 0 \
    bash "$REPORT" --repo-root "$REAL_ROOT" --summary
  assert_stdout_contains "live report prints the auto-discovery aggregate" \
    "auto-discovery-required:" bash "$REPORT" --repo-root "$REAL_ROOT" --summary
else
  echo "SKIP: real skills/INVENTORY.md absent (downstream install) — live (a) check skipped"
fi

echo "ALL PASS: skill-description-load-selftest"
