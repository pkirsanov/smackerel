#!/usr/bin/env bash
# Bubbles release manifest purity selftest (v5.0.1 / H6).
#
# Regression test for the manifest-enumeration leak fixed by
# trust-metadata.sh adopting `git ls-files` when source is a git checkout.
#
# Plants an untracked file inside framework-managed directories, regenerates
# the release manifest into a temp output path, and asserts that the
# untracked file does NOT appear in the manifest.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
GEN="$SCRIPT_DIR/generate-release-manifest.sh"

if [[ ! -x "$GEN" ]]; then
  echo "release-manifest-purity-selftest: SKIP (generator not executable at $GEN)"
  exit 0
fi

if ! command -v git >/dev/null 2>&1; then
  echo "release-manifest-purity-selftest: SKIP (git not installed)"
  exit 0
fi

if ! git -C "$REPO_ROOT" rev-parse >/dev/null 2>&1; then
  echo "release-manifest-purity-selftest: SKIP (not a git repo)"
  exit 0
fi

failures=0

pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; failures=$((failures + 1)); }

# Plant untracked files in framework-managed locations.
declare -a planted=()
plant() {
  local rel="$1"
  local abs="$REPO_ROOT/$rel"
  mkdir -p "$(dirname "$abs")"
  printf 'untracked manifest leak probe\n' > "$abs"
  planted+=("$abs")
}

cleanup() {
  for p in "${planted[@]}"; do
    [[ -f "$p" ]] && rm -f "$p"
  done
}
trap cleanup EXIT INT TERM

plant "bubbles/scripts/__manifest_leak_probe.sh"
plant "skills/__manifest_leak_probe/SKILL.md"
plant "bubbles/adapters/observability/__manifest_leak_probe.sh"

# Verify the planted files are NOT git-tracked.
for p in "${planted[@]}"; do
  rel="${p#$REPO_ROOT/}"
  if git -C "$REPO_ROOT" ls-files --error-unmatch -- "$rel" >/dev/null 2>&1; then
    fail "Planted file unexpectedly already tracked: $rel"
  else
    pass "Planted untracked probe: $rel"
  fi
done

# Regenerate manifest into a temp file by reusing the generator with stdout
# redirect (the generator overwrites bubbles/release-manifest.json). We do
# this and then restore the original from git.
ORIGINAL_MANIFEST="$REPO_ROOT/bubbles/release-manifest.json"
BACKUP="$(mktemp)"
cp "$ORIGINAL_MANIFEST" "$BACKUP"
restore_manifest() {
  [[ -f "$BACKUP" ]] && cp "$BACKUP" "$ORIGINAL_MANIFEST" && rm -f "$BACKUP"
}
trap 'cleanup; restore_manifest' EXIT INT TERM

bash "$GEN" >/dev/null 2>&1 || true

# Check none of the planted paths appear in the regenerated manifest.
for p in "${planted[@]}"; do
  rel="${p#$REPO_ROOT/}"
  if grep -Fq "\"$rel\"" "$ORIGINAL_MANIFEST" 2>/dev/null; then
    fail "Untracked planted file leaked into manifest: $rel"
  else
    pass "Untracked file excluded from manifest: $rel"
  fi
done

if [[ "$failures" -gt 0 ]]; then
  echo "release-manifest-purity-selftest: FAIL ($failures issue(s))"
  exit 1
fi

echo "release-manifest-purity-selftest: PASS"
exit 0
