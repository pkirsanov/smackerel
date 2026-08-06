#!/usr/bin/env bash
# bubbles/scripts/gate-vintage-annotate.sh
#
# Derives each gate's introducing framework version from git history and writes
# `since` / `sinceDate` into bubbles/registry/gates.yaml (IMP-036 SCOPE-8).
#
# WHY THIS EXISTS
# Gate ids grew from 71 to 134 in four months. Nothing recorded WHEN a gate
# arrived, so a catalogue-wide sweep could reopen a spec that was certified
# before the gate existed. About 176 specs across six repos carry reopen /
# recertification language from exactly that pattern. Re-litigating closed work
# when only the rules changed is the least defensible cost in the system.
#
# The vintage is DERIVED, never invented: for each gate id, the first commit
# that introduced its `  Gxxx:` key in the registry, and the VERSION file
# content at that commit. A gate whose history cannot be resolved is left
# unannotated rather than given a guessed value.
#
# Idempotent. Re-run after adding a gate; existing annotations are refreshed
# from history, so a hand-edited value does not survive and cannot drift.
#
# Usage:
#   bash bubbles/scripts/gate-vintage-annotate.sh [--check]
#
# Exit codes:
#   0 = annotated (or --check found the file already fresh)
#   1 = --check found the file stale
#   2 = usage error, or not a git checkout

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
GATES_FILE="${BUBBLES_GATES_FILE:-$ROOT_DIR/bubbles/registry/gates.yaml}"

CHECK_ONLY="false"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --check) CHECK_ONLY="true" ;;
    -h|--help) sed -n '2,28p' "${BASH_SOURCE[0]}"; exit 0 ;;
    --skip*|--force*|--ignore*)
      printf 'gate-vintage-annotate: bypass-shaped flag "%s" is not supported\n' "$1" >&2; exit 2 ;;
    *) printf 'gate-vintage-annotate: unknown argument "%s"\n' "$1" >&2; exit 2 ;;
  esac
  shift
done

[[ -f "$GATES_FILE" ]] || { printf 'gate-vintage-annotate: gates file not found: %s\n' "$GATES_FILE" >&2; exit 2; }
command -v git >/dev/null 2>&1 || { printf 'gate-vintage-annotate: git is required\n' >&2; exit 2; }
git -C "$ROOT_DIR" rev-parse --is-inside-work-tree >/dev/null 2>&1 || {
  printf 'gate-vintage-annotate: not a git checkout: %s\n' "$ROOT_DIR" >&2; exit 2; }

# Resolve one gate's introducing commit, version and date.
resolve_vintage() {
  local gate="$1" first sha ver day
  first="$(git -C "$ROOT_DIR" log --reverse --format='%H' -S"  ${gate}:" \
    -- bubbles/registry/gates.yaml 2>/dev/null | head -1)"
  [[ -n "$first" ]] || return 1
  sha="$first"
  ver="$(git -C "$ROOT_DIR" show "$sha:VERSION" 2>/dev/null | tr -d ' \n')"
  day="$(git -C "$ROOT_DIR" log -1 --format=%ad --date=short "$sha" 2>/dev/null)"
  [[ -n "$ver" && -n "$day" ]] || return 1
  printf '%s\t%s' "$ver" "$day"
}

gates="$(grep -oE '^  G[0-9]{3}:' "$GATES_FILE" | tr -d ' :' | LC_ALL=C sort -u)"
[[ -n "$gates" ]] || { printf 'gate-vintage-annotate: no gate ids found in %s\n' "$GATES_FILE" >&2; exit 2; }

tmp="$(mktemp)" || exit 2
trap 'rm -f "$tmp" "$tmp.map"' EXIT INT TERM

resolved=0
unresolved=""
: >"$tmp.map"
while IFS= read -r g; do
  [[ -n "$g" ]] || continue
  if v="$(resolve_vintage "$g")"; then
    printf '%s\t%s\n' "$g" "$v" >>"$tmp.map"
    resolved=$((resolved + 1))
  else
    unresolved="$unresolved $g"
  fi
done <<<"$gates"

# Rewrite: insert or refresh `since` / `sinceDate` immediately after each gate key.
awk -v mapfile="$tmp.map" '
  BEGIN {
    while ((getline line < mapfile) > 0) {
      split(line, f, "\t"); ver[f[1]] = f[2]; day[f[1]] = f[3]
    }
  }
  # Drop any existing annotation so a re-run refreshes rather than duplicates.
  /^    (since|sinceDate):/ { next }
  {
    print
    if ($0 ~ /^  G[0-9][0-9][0-9]:$/) {
      g = $0; sub(/^  /, "", g); sub(/:$/, "", g)
      if (g in ver) {
        printf "    since: \"%s\"\n", ver[g]
        printf "    sinceDate: \"%s\"\n", day[g]
      }
    }
  }
' "$GATES_FILE" >"$tmp"

if [[ "$CHECK_ONLY" == "true" ]]; then
  if cmp -s "$tmp" "$GATES_FILE"; then
    printf '[gate-vintage-annotate] fresh - %d gate(s) annotated\n' "$resolved"
    exit 0
  fi
  printf '[gate-vintage-annotate] STALE - run: bash bubbles/scripts/gate-vintage-annotate.sh\n' >&2
  exit 1
fi

cat "$tmp" >"$GATES_FILE"
printf '[gate-vintage-annotate] annotated %d gate(s) with since/sinceDate\n' "$resolved"
if [[ -n "$unresolved" ]]; then
  printf '[gate-vintage-annotate] unresolved (left unannotated rather than guessed):%s\n' "$unresolved"
fi
exit 0
