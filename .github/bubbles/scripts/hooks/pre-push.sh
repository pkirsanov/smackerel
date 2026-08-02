#!/usr/bin/env bash
#
# Bubbles source-repo pre-push hook (v5.0.1).
#
# Installed by `bash bubbles/scripts/install-bubbles-hooks.sh` for
# framework maintainers. Runs framework-validate + release-check
# before any push to origin. NO bypass flags.
#
# This is the framework eating its own dog food: the framework's
# release process refuses pushes that would ship framework drift.

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
SCRIPT_DIR="$REPO_ROOT/bubbles/scripts"
source "$SCRIPT_DIR/hooks/git-env-sanitize.sh"

# Git exports repository-local variables to hooks. Clear them before the
# framework selftests create nested repositories, or their Git commands can
# mutate the repository being pushed instead of their disposable fixtures.
bubbles_unset_git_local_env
cd "$REPO_ROOT"

echo "🫧 bubbles pre-push: running framework validation..."

if [[ ! -x "$SCRIPT_DIR/framework-validate.sh" ]]; then
  echo "⚠️  framework-validate.sh not found or not executable; skipping"
  exit 0
fi

# Proportional validation (IMP-100 Phase 5 / R8): a maintainer may opt into the
# fast structural CORE tier for routine local pushes via BUBBLES_PREPUSH_TIER=core.
# The DEFAULT stays the FULL validate + release-check — the release gate is never
# silently weakened (CI does not run the full suite). A core-tier push MUST be
# followed by a full validate + release-check before cutting a release.
PREPUSH_TIER="${BUBBLES_PREPUSH_TIER:-full}"

# Per-process paths: a fixed name is silently overwritten by any concurrent push on the same machine.
PREPUSH_VALIDATE_LOG="/tmp/bubbles-pre-push-validate.$$.log"
PREPUSH_RELEASE_LOG="/tmp/bubbles-pre-push-release.$$.log"

if [[ "$PREPUSH_TIER" == "core" ]]; then
  echo "🫧 bubbles pre-push: tier=core (fast structural gate — full validate + release-check still required before a release)"
  if ! bash "$SCRIPT_DIR/framework-validate.sh" --tier=core >"$PREPUSH_VALIDATE_LOG" 2>&1; then
    echo "❌ framework-validate (core tier) failed. Full log: $PREPUSH_VALIDATE_LOG"
    echo "    Tail:"
    tail -30 "$PREPUSH_VALIDATE_LOG" | sed 's/^/      /'
    echo ""
    echo "    Fix the failures and retry the push. There is no bypass."
    exit 1
  fi
  echo "✅ framework-validate (core tier) passed — run a FULL validate + release-check before cutting a release."
  exit 0
fi

if ! bash "$SCRIPT_DIR/framework-validate.sh" >"$PREPUSH_VALIDATE_LOG" 2>&1; then
  echo "❌ framework-validate failed. Full log: $PREPUSH_VALIDATE_LOG"
  echo "    Tail:"
  tail -30 "$PREPUSH_VALIDATE_LOG" | sed 's/^/      /'
  echo ""
  echo "    Fix the failures and retry the push. There is no bypass."
  exit 1
fi
echo "✅ framework-validate passed"

if [[ -x "$SCRIPT_DIR/release-check.sh" ]]; then
  echo "🫧 bubbles pre-push: running release-check..."
  if ! bash "$SCRIPT_DIR/release-check.sh" >"$PREPUSH_RELEASE_LOG" 2>&1; then
    echo "❌ release-check failed. Full log: $PREPUSH_RELEASE_LOG"
    echo "    Tail:"
    tail -30 "$PREPUSH_RELEASE_LOG" | sed 's/^/      /'
    echo ""
    echo "    Fix the failures and retry the push. There is no bypass."
    exit 1
  fi
  echo "✅ release-check passed"
fi

exit 0
