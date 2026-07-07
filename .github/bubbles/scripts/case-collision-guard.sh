#!/usr/bin/env bash
# case-collision-guard.sh — fail-loud guard against case-only duplicate paths in
# a git repository's tracked file set.
#
# On case-insensitive filesystems (macOS, Windows) two tracked paths that differ
# ONLY by letter case — e.g. templates/AGENTS.md.tmpl vs templates/agents.md.tmpl
# — collapse to a SINGLE physical file. The git index still carries two entries
# (often pointing at the same blob), so on those platforms one file silently
# shadows the other, `git checkout` / rebase / `git status` misbehave, and
# DERIVED artifacts (release manifest, checksums) diverge between a
# case-sensitive and a case-insensitive checkout. This guard scans
# `git ls-files` for any such case-insensitive duplicate and fails loud.
#
# Exit 0 = no case-insensitive duplicate tracked paths (or the target is not a
#          git work tree / git is unavailable — nothing tracked to scan).
# Exit 1 = one or more case-collision groups found (listed on stderr).
# Exit 2 = usage error (bad flag / missing repo root).
#
# There is NO bypass flag. Rename or de-duplicate the offending path; never skip
# the check. Portable across GNU (Linux/WSL) and BSD (macOS) userland: the scan
# relies only on POSIX awk (tolower / printf / split) and `LC_ALL=C sort` for a
# stable, platform-independent ordering.
#
# Origin: IMP-017 (template case-collision fix + prevention).

set -euo pipefail

usage() {
  cat <<'EOF'
Usage: case-collision-guard.sh [--repo-root DIR]

Scan a git repository's tracked files (git ls-files) for any two paths that
differ ONLY by case (case-insensitive duplicates) and fail loud if any exist.

Options:
  --repo-root DIR   Repository root to scan. Default: inferred from this
                    script's location (framework source or downstream install
                    tree), else the current directory.
  -h, --help        Show this help and exit 0.

Exit codes:
  0  no case collisions (or DIR is not a git work tree)
  1  case collision(s) found
  2  usage error
EOF
}

REPO_ROOT=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h | --help)
      usage
      exit 0
      ;;
    --repo-root)
      shift
      [[ $# -gt 0 ]] || {
        echo "[case-collision-guard][USAGE] --repo-root requires a directory argument" >&2
        exit 2
      }
      REPO_ROOT="$1"
      shift
      ;;
    --repo-root=*)
      REPO_ROOT="${1#*=}"
      shift
      ;;
    *)
      echo "[case-collision-guard][USAGE] unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

# Resolve REPO_ROOT: explicit flag > inferred from script location > CWD.
if [[ -z "$REPO_ROOT" ]]; then
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" &&
    "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
    # downstream install tree: .github/bubbles/scripts/ -> repo root
    REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
  elif [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" ]]; then
    # framework source tree: bubbles/scripts/ -> repo root
    REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
  else
    REPO_ROOT="$(pwd)"
  fi
fi

if [[ ! -d "$REPO_ROOT" ]]; then
  echo "[case-collision-guard][USAGE] repo root not found: $REPO_ROOT" >&2
  exit 2
fi

# Not a git work tree (or git unavailable) -> nothing tracked to scan; pass.
if ! command -v git >/dev/null 2>&1; then
  echo "[case-collision-guard] git not found on PATH; nothing to scan."
  exit 0
fi
if ! git -C "$REPO_ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "[case-collision-guard] $REPO_ROOT is not a git work tree; nothing to scan."
  exit 0
fi

# Scan tracked paths for case-insensitive duplicates. Deterministic:
#   1. emit "lowercasekey<TAB>path" for every tracked path (POSIX awk tolower())
#   2. LC_ALL=C sort so rows sharing a key are adjacent + ordering is stable
#   3. adjacent-group scan; any key mapping to >1 tracked path is a collision.
# The final awk exits 3 (sentinel) when at least one collision group is printed,
# so a real awk/pipeline error (exit 1/2) is never confused with "collision
# found" (3).
collision_report=""
scan_rc=0
collision_report="$(
  git -C "$REPO_ROOT" ls-files |
    LC_ALL=C awk '{ key = tolower($0); print key "\t" $0 }' |
    LC_ALL=C sort |
    LC_ALL=C awk '
      BEGIN { FS = "\t" }
      function flush(   i) {
        if (count > 1) {
          found = 1
          printf("  case-insensitive duplicate (%d tracked paths differ only by case):\n", count)
          for (i = 1; i <= count; i++) printf("    %s\n", group[i])
        }
      }
      {
        if ($1 != curkey) { flush(); curkey = $1; count = 0; split("", group) }
        count++
        group[count] = $2
      }
      END { flush(); if (found) exit 3 }
    '
)" || scan_rc=$?

if [[ "$scan_rc" -eq 0 ]]; then
  tracked_count="$(git -C "$REPO_ROOT" ls-files | wc -l | tr -d '[:space:]')"
  echo "[case-collision-guard] OK — no case-insensitive duplicate paths among ${tracked_count} tracked file(s)."
  exit 0
elif [[ "$scan_rc" -eq 3 ]]; then
  echo "[case-collision-guard] FAIL — case-insensitive duplicate tracked paths found:" >&2
  printf '%s\n' "$collision_report" >&2
  echo "[case-collision-guard] These paths collapse to ONE physical file on case-insensitive filesystems (macOS/Windows)." >&2
  echo "[case-collision-guard] Remove the duplicate with an exact-case op, e.g.:" >&2
  echo "[case-collision-guard]   git -c core.ignorecase=false rm --cached <duplicate-path>" >&2
  exit 1
else
  echo "[case-collision-guard][ERROR] tracked-file scan failed unexpectedly (rc=$scan_rc)." >&2
  exit 2
fi
