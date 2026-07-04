#!/usr/bin/env bash
# inventory-parity-check-selftest.sh — hermetic selftest for
# inventory-parity-check.sh. Builds throwaway skills/ + INVENTORY.md fixtures.
#
# Cases:
#   (a) matched inventory (2 skills, 2 rows)                → exit 0
#   (b) a real skill dir missing its INVENTORY row          → exit 1, names it
#   (c) an INVENTORY row for a non-existent skill dir       → exit 1, names it
#   (d) a `__*` probe dir present but NOT inventoried       → exit 0 (ignored)
#   (e) usage errors (missing INVENTORY.md; -h)             → exit 2

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHECK="$SCRIPT_DIR/inventory-parity-check.sh"

[[ -x "$CHECK" ]] || {
  echo "FAIL: $CHECK not executable" >&2
  exit 1
}

TMP="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-selftest-inv-parity.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

reset_fixture() {
  rm -rf "$TMP/skills"
  mkdir -p "$TMP/skills"
}

# make_skill <name> — a skills/<name>/ dir with a minimal SKILL.md.
make_skill() {
  mkdir -p "$TMP/skills/$1"
  printf -- '---\nname: %s\n---\n# %s\n' "$1" "$1" >"$TMP/skills/$1/SKILL.md"
}

# write_inventory <name...> — one inventory table row per name, plus header
# prose and a decision-matrix-style row that MUST be ignored by the guard.
write_inventory() {
  {
    echo "# Bubbles Skills Inventory"
    echo
    echo "| Status | Meaning |"
    echo "|---|---|"
    echo "| **KEEP** | decision-matrix row — first cell is not a backtick token, must be ignored. |"
    echo
    echo "| Skill | LOC | Status | Notes |"
    echo "|---|---:|---|---|"
    for s in "$@"; do
      echo "| \`$s\` | 42 | KEEP | fixture row for \`$s\`. |"
    done
    echo
    echo "- **$# skills**."
  } >"$TMP/skills/INVENTORY.md"
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

# assert_stderr_names <desc> <needle> <cmd...>
assert_stderr_names() {
  local desc="$1" needle="$2"
  shift 2
  local out
  out="$("$@" 2>&1 || true)"
  if grep -q "$needle" <<<"$out"; then
    echo "PASS: $desc (mentions '$needle')"
  else
    echo "FAIL: $desc (stderr did not mention '$needle')" >&2
    echo "$out" >&2
    exit 1
  fi
}

# (a) matched inventory → exit 0
reset_fixture
make_skill bubbles-alpha
make_skill bubbles-beta
write_inventory bubbles-alpha bubbles-beta
assert_exit "matched inventory passes" 0 bash "$CHECK" "$TMP"

# (b) a real skill dir missing its INVENTORY row → exit 1, names it
reset_fixture
make_skill bubbles-alpha
make_skill bubbles-orphan-dir
write_inventory bubbles-alpha
assert_exit "real skill missing its row fails" 1 bash "$CHECK" "$TMP"
assert_stderr_names "names the un-inventoried skill" "bubbles-orphan-dir" bash "$CHECK" "$TMP"

# (c) an INVENTORY row for a non-existent skill dir → exit 1, names it
reset_fixture
make_skill bubbles-alpha
write_inventory bubbles-alpha bubbles-ghost-row
assert_exit "orphan inventory row fails" 1 bash "$CHECK" "$TMP"
assert_stderr_names "names the orphan inventory row" "bubbles-ghost-row" bash "$CHECK" "$TMP"

# (d) a `__*` probe dir present but NOT inventoried → exit 0 (probe ignored).
#     If the guard did NOT exclude `__*`, the probe would be flagged as a
#     missing row and this would exit 1 — so exit 0 proves the exclusion.
reset_fixture
make_skill bubbles-alpha
make_skill __manifest_leak_probe
write_inventory bubbles-alpha
assert_exit "__* probe dir is ignored (not flagged)" 0 bash "$CHECK" "$TMP"

# (e) usage errors → exit 2
reset_fixture
make_skill bubbles-alpha # skills/ exists but no INVENTORY.md written
assert_exit "missing INVENTORY.md is a usage error" 2 bash "$CHECK" "$TMP"
assert_exit "-h prints usage and exits 2" 2 bash "$CHECK" -h

echo "All inventory-parity-check selftests passed."
