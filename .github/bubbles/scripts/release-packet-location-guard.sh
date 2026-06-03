#!/usr/bin/env bash
# release-packet-location-guard.sh — enforces canonical release-packet location.
#
# Canonical location (per bubbles.releases.agent.md):
#   docs/releases/<phase>/{vision,features,actions,business-plan,
#                          deployment,marketing,monetization,ops-scalability}.md
#
# Forbidden alternatives include (but are not limited to):
#   specs/_ops/RELEASE-*/<file>.md
#   specs/releases/<phase>/<file>.md
#   docs/RELEASE-*/<file>.md
#   any path containing "release" (case-insensitive) that is NOT under docs/releases/
#
# Detection rule:
#   - Find every file whose basename matches one of the 8 canonical release-packet docs.
#   - Flag it if its path contains "release" (case-insensitive) AND it is not under
#     a path of the form .../docs/releases/<phase>/<basename>.
#   - This narrow filter avoids false positives on unrelated files (e.g. a generic
#     features.md sitting in some other directory of a downstream repo).
#
# Exit 0 = clean. Exit 1 = misplaced release-packet doc(s) found.
# No --skip / --force / --ignore flag exists by design.

set -euo pipefail

REPO_ROOT="${1:-.}"

if [[ ! -d "$REPO_ROOT" ]]; then
  echo "[release-packet-location-guard][ERROR] repo root not found: $REPO_ROOT" >&2
  exit 2
fi

REPO_ROOT_ABS="$(cd "$REPO_ROOT" && pwd -P)"

CANONICAL_DOCS=(
  "vision.md"
  "features.md"
  "actions.md"
  "business-plan.md"
  "deployment.md"
  "marketing.md"
  "monetization.md"
  "ops-scalability.md"
)

# Build the find -name expression: \( -name a -o -name b -o ... \)
find_name_expr=()
first=1
for name in "${CANONICAL_DOCS[@]}"; do
  if [[ $first -eq 1 ]]; then
    find_name_expr+=( -name "$name" )
    first=0
  else
    find_name_expr+=( -o -name "$name" )
  fi
done

# Canonical path regex anchored at repo root:
#   ${REPO_ROOT}/docs/releases/<phase>/<canonical-basename>
# <phase> may not contain '/'.
canonical_re="^${REPO_ROOT_ABS}/docs/releases/[^/]+/(vision|features|actions|business-plan|deployment|marketing|monetization|ops-scalability)\.md$"

# "Looks like a release packet" filter — path (case-insensitive) contains 'release'.
# This is the false-positive filter requested in the design.
release_path_re='[Rr][Ee][Ll][Ee][Aa][Ss][Ee]'

violations=()

while IFS= read -r -d '' path; do
  # Skip anything under .git, node_modules, .venv, etc.
  case "$path" in
    */.git/*|*/node_modules/*|*/.venv/*|*/venv/*|*/.tox/*|*/dist/*|*/build/*)
      continue
      ;;
  esac

  # Only flag files whose path looks release-related.
  if [[ ! "$path" =~ $release_path_re ]]; then
    continue
  fi

  # Allow canonical location.
  if [[ "$path" =~ $canonical_re ]]; then
    continue
  fi

  violations+=("$path")
done < <(find "$REPO_ROOT_ABS" -type f \( "${find_name_expr[@]}" \) -print0 2>/dev/null)

if [[ ${#violations[@]} -gt 0 ]]; then
  {
    echo "[release-packet-location-guard][ERROR] release-packet misplacement detected"
    echo "Canonical location: docs/releases/<phase>/<canonical-doc>.md"
    echo "Offending paths (must be moved or renamed):"
    for v in "${violations[@]}"; do
      echo "  - ${v#$REPO_ROOT_ABS/}"
    done
    echo ""
    echo "Allowed canonical docs per phase:"
    for name in "${CANONICAL_DOCS[@]}"; do
      echo "  - docs/releases/<phase>/$name"
    done
    echo ""
    echo "Forbidden: state.json, README.md inside a release-packet directory."
    echo "Owner: bubbles.releases (Sonny \"Iron Lung\" Smith)."
  } >&2
  exit 1
fi

echo "[release-packet-location-guard] OK (no misplaced release-packet docs)"
exit 0
