#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  echo "release-check is for the Bubbles source repo, not an installed downstream framework layer." >&2
  exit 1
fi
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

failures=0

run_check() {
  local label="$1"
  shift

  echo "==> $label"
  if "$@"; then
    echo "PASS: $label"
  else
    echo "FAIL: $label"
    failures=$((failures + 1))
  fi
  echo
}

check_required_files() {
  local missing=0
  local required_files=(
    "$REPO_ROOT/README.md"
    "$REPO_ROOT/CHANGELOG.md"
    "$REPO_ROOT/docs/CHEATSHEET.md"
    "$REPO_ROOT/docs/its-not-rocket-appliances.html"
    "$REPO_ROOT/docs/generated/competitive-capabilities.md"
    "$REPO_ROOT/docs/generated/issue-status.md"
    "$REPO_ROOT/docs/guides/AGENT_MANUAL.md"
    "$REPO_ROOT/docs/guides/INSTALLATION.md"
    "$REPO_ROOT/docs/guides/CONTROL_PLANE_DESIGN.md"
    "$REPO_ROOT/docs/guides/CONTROL_PLANE_SCHEMAS.md"
    "$REPO_ROOT/docs/recipes/framework-ops.md"
    "$REPO_ROOT/bubbles/capability-ledger.yaml"
    "$REPO_ROOT/bubbles/release-manifest.json"
    "$REPO_ROOT/bubbles/action-risk-registry.yaml"
    "$REPO_ROOT/bubbles/scripts/repo-readiness.sh"
    "$REPO_ROOT/install.sh"
    "$REPO_ROOT/VERSION"
  )

  for required_file in "${required_files[@]}"; do
    if [[ ! -f "$required_file" ]]; then
      echo "Missing required release file: $required_file" >&2
      missing=1
    fi
  done

  return "$missing"
}

check_stray_release_files() {
  local found=0
  while IFS= read -r stray_file; do
    [[ -n "$stray_file" ]] || continue
    echo "Unexpected temporary or backup file: $stray_file" >&2
    found=1
  done < <(find "$REPO_ROOT" \
    -path "$REPO_ROOT/.git" -prune -o \
    \( -name '*.tmp' -o -name '*.bak' -o -name '*.orig' -o -name '*~' \) -print)

  if [[ "$found" -eq 1 ]]; then
    return 1
  fi
}

echo "Bubbles Release Check"
echo "Repository: $REPO_ROOT"
echo

run_check "Framework validation" bash "$SCRIPT_DIR/framework-validate.sh"
run_check "Capability ledger docs freshness" bash "$SCRIPT_DIR/generate-capability-ledger-docs.sh" --check
run_check "Release manifest freshness" bash "$SCRIPT_DIR/generate-release-manifest.sh" --check
run_check "Required release files" check_required_files
run_check "No stray temp or backup files" check_stray_release_files

if [[ "$failures" -gt 0 ]]; then
  echo "Release check failed with $failures failing check(s)."
  exit 1
fi

echo "Release check passed."