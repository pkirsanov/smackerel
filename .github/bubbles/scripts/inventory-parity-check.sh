#!/usr/bin/env bash
# inventory-parity-check.sh — parity guard between the real skill directories on
# disk and the skill rows recorded in skills/INVENTORY.md, so the inventory
# count/rows cannot silently drift away from the filesystem (IMP-005 SCOPE-1).
#
# A "real skill" is any skills/<name>/ directory that contains a SKILL.md,
# EXCLUDING transient probe directories whose name begins with `__` (e.g. the
# gitignored `__manifest_leak_probe/` planted+removed by
# release-manifest-purity-selftest). Probe dirs are never inventoried.
#
# An "inventoried skill" is the backtick-quoted name in the FIRST column of each
# row of the inventory table in skills/INVENTORY.md (prose backticks and the
# decision-matrix rows are ignored — only a leading `| `<name>` ` cell counts).
#
# Exit 0 = the two sets match exactly.
# Exit 1 = drift (fail-loud, offending names listed): a real skill has no
#          INVENTORY row, OR INVENTORY references a skill dir that does not exist.
# Exit 2 = usage / malformed input (missing skills dir or INVENTORY.md, or -h).
#
# There is NO bypass flag. Reconcile skills/INVENTORY.md; never skip the check.

set -euo pipefail

usage() {
  echo "Usage: bash inventory-parity-check.sh [REPO_ROOT]" >&2
  echo "  REPO_ROOT defaults to the framework repo root inferred from this script's location." >&2
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 2
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -n "${1:-}" ]]; then
  REPO_ROOT="$1"
elif [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  # downstream install tree: .github/bubbles/scripts/ -> repo root
  REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
else
  # framework source tree: bubbles/scripts/ -> repo root
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

SKILLS_DIR="$REPO_ROOT/skills"
INVENTORY="$SKILLS_DIR/INVENTORY.md"

if [[ ! -d "$SKILLS_DIR" ]]; then
  echo "[inventory-parity-check][USAGE] skills/ directory not found at: $SKILLS_DIR" >&2
  exit 2
fi
if [[ ! -f "$INVENTORY" ]]; then
  echo "[inventory-parity-check][USAGE] skills/INVENTORY.md not found at: $INVENTORY" >&2
  exit 2
fi

disk_skills="$(mktemp)"
inv_skills="$(mktemp)"
trap 'rm -f "$disk_skills" "$inv_skills"' EXIT

# --- Set 1: real skill dirs on disk (have SKILL.md, exclude `__*` probe dirs) ---
shopt -s nullglob
for d in "$SKILLS_DIR"/*/; do
  name="$(basename "$d")"
  case "$name" in
    __*) continue ;; # transient selftest probe dir — never inventoried
  esac
  if [[ -f "$d/SKILL.md" ]]; then
    echo "$name"
  fi
done | LC_ALL=C sort -u >"$disk_skills"

# --- Set 2: skill rows in INVENTORY.md (first backtick token of each table row) ---
# Rows look like: | `bubbles-anti-fabrication` | 50 | KEEP | ... |
# `grep -o` anchored at `^|` extracts ONLY the first cell, so later Notes-column
# backticks and the `| **KEEP** | ...` decision-matrix rows are ignored.
{ grep -oE '^\|[[:space:]]*`[^`]+`' "$INVENTORY" || true; } \
  | sed -E 's/^\|[[:space:]]*`([^`]+)`/\1/' \
  | LC_ALL=C sort -u >"$inv_skills"

FAILED=0

missing_rows="$(comm -23 "$disk_skills" "$inv_skills")"
if [[ -n "$missing_rows" ]]; then
  FAILED=1
  echo "[inventory-parity-check][ERROR] real skill(s) with NO skills/INVENTORY.md row:" >&2
  while IFS= read -r s; do echo "  - $s" >&2; done <<<"$missing_rows"
fi

orphan_rows="$(comm -13 "$disk_skills" "$inv_skills")"
if [[ -n "$orphan_rows" ]]; then
  FAILED=1
  echo "[inventory-parity-check][ERROR] skills/INVENTORY.md row(s) with NO skills/<name>/ directory:" >&2
  while IFS= read -r s; do echo "  - $s" >&2; done <<<"$orphan_rows"
fi

if [[ "$FAILED" -ne 0 ]]; then
  echo "[inventory-parity-check] FAIL — reconcile skills/INVENTORY.md with the filesystem." >&2
  exit 1
fi

n="$(wc -l <"$disk_skills" | tr -d ' ')"
echo "[inventory-parity-check] OK — $n real skill(s), all inventoried; no orphan rows."
exit 0
