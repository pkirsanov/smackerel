#!/usr/bin/env bash
#
# Install Bubbles source-repo pre-push hook for framework maintainers.
# Idempotent. If a pre-push hook already exists, this script INJECTS
# the framework-validate + release-check pass before the final exit
# instead of replacing the existing hook.

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
HOOK_SRC="$REPO_ROOT/bubbles/scripts/hooks/pre-push.sh"
HOOK_DST="$REPO_ROOT/.git/hooks/pre-push"
MARKER='# === bubbles framework-validate guard (installed by install-bubbles-hooks.sh) ==='

if [[ ! -f "$HOOK_SRC" ]]; then
  echo "install-bubbles-hooks: source hook missing at $HOOK_SRC" >&2
  exit 1
fi

mkdir -p "$REPO_ROOT/.git/hooks"

if [[ ! -f "$HOOK_DST" ]]; then
  cp "$HOOK_SRC" "$HOOK_DST"
  chmod +x "$HOOK_DST"
  echo "Installed pre-push hook → .git/hooks/pre-push"
  echo "Runs framework-validate + release-check on every push. No bypass."
  exit 0
fi

if grep -Fq "$MARKER" "$HOOK_DST"; then
  echo "Pre-push hook already wired with bubbles framework guard. Nothing to do."
  exit 0
fi

# Inject the guard block at the END of the hook, before any final exit.
{
  echo ""
  echo "$MARKER"
  echo "if [[ -x \"\$(git rev-parse --show-toplevel)/bubbles/scripts/hooks/pre-push.sh\" ]]; then"
  echo "  bash \"\$(git rev-parse --show-toplevel)/bubbles/scripts/hooks/pre-push.sh\" || exit 1"
  echo "fi"
} >> "$HOOK_DST"

chmod +x "$HOOK_DST"
echo "Appended bubbles framework guard to existing .git/hooks/pre-push"
echo "Runs framework-validate + release-check after existing checks. No bypass."

