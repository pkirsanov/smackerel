#!/usr/bin/env bash
#
# Bubbles source-repo pre-push hook (v5.0.1).
#
# Installed by `bash bubbles/scripts/install-bubbles-hooks.sh` for
# framework maintainers. Runs the fast structural CORE tier by default;
# the full release gate runs in CI. NO bypass flags.
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

# Proportional validation (IMP-100 Phase 5 / R8). The DEFAULT is the fast
# structural CORE tier (~90s, 16 checks); `BUBBLES_PREPUSH_TIER=full` runs the
# whole release gate locally and is what a release cut should use.
#
# Core-by-default is safe because the full gate is NOT skipped, only relocated:
# .github/workflows/agnosticity.yml runs `cli.sh release-check` on every pull
# request and every push to main, on Linux AND macOS. A local full run for each
# routine push re-computes a verdict CI produces anyway, and a ~30-minute hook
# is the strongest practical incentive toward the bypass behaviour this
# framework exists to prevent.
PREPUSH_TIER="${BUBBLES_PREPUSH_TIER:-core}"

# Per-process paths: a fixed name is silently overwritten by any concurrent push on the same machine.
PREPUSH_VALIDATE_LOG="/tmp/bubbles-pre-push-validate.$$.log"
PREPUSH_RELEASE_LOG="/tmp/bubbles-pre-push-release.$$.log"

if [[ "$PREPUSH_TIER" == "core" ]]; then
  echo "🫧 bubbles pre-push: tier=core (fast structural gate — the full release gate runs in CI, and BUBBLES_PREPUSH_TIER=full runs it here)"
  if ! bash "$SCRIPT_DIR/framework-validate.sh" --tier=core >"$PREPUSH_VALIDATE_LOG" 2>&1; then
    echo "❌ framework-validate (core tier) failed. Full log: $PREPUSH_VALIDATE_LOG"
    echo "    Tail:"
    tail -30 "$PREPUSH_VALIDATE_LOG" | sed 's/^/      /'
    echo ""
    echo "    Fix the failures and retry the push. There is no bypass."
    exit 1
  fi
  echo "✅ framework-validate (core tier) passed — CI runs the full release-check; use BUBBLES_PREPUSH_TIER=full before cutting a release."
  exit 0
fi

# release-check.sh runs framework-validate.sh as its OWN first check, so
# invoking both here ran the entire suite twice for one verdict — nothing can
# change the tree between them. Prefer release-check, which is a strict
# superset; fall back to a bare validate only when release-check is absent.
if [[ -x "$SCRIPT_DIR/release-check.sh" ]]; then
  echo "🫧 bubbles pre-push: running release-check (framework-validate runs inside it)..."
  if ! bash "$SCRIPT_DIR/release-check.sh" >"$PREPUSH_RELEASE_LOG" 2>&1; then
    echo "❌ release-check failed. Full log: $PREPUSH_RELEASE_LOG"
    echo "    Tail:"
    tail -30 "$PREPUSH_RELEASE_LOG" | sed 's/^/      /'
    echo ""
    echo "    Fix the failures and retry the push. There is no bypass."
    exit 1
  fi
  echo "✅ release-check passed (framework-validate included)"
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

exit 0
