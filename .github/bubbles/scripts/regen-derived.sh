#!/usr/bin/env bash
# File: regen-derived.sh
#
# One-shot, dependency-ordered regeneration of every derived artifact whose
# freshness `release-check.sh` enforces. Adding a gate / agent, or bumping
# VERSION, stales one or more of: framework-stats (README, CHEATSHEET, the html
# cheatsheet, docs/generated/framework-stats.*), the cheatsheet, the
# capability-ledger docs, and the release manifest. They MUST be regenerated in
# dependency order, and the RELEASE MANIFEST IS LAST because README / CHEATSHEET
# / html are manifest-tracked, so regenerating any of them invalidates the
# manifest.
#
# This wrapper removes the recurring "which generators, in what order?" trap (the
# new-gate authoring checklist re-derives it every release; a manifest-before-
# stats mistake blocks the push at release-check with stale checksums). It is
# idempotent: on an already-fresh tree it rewrites identical bytes and the final
# verification pass is a no-op.
#
# Usage: regen-derived.sh [--check-only | -h | --help]
#   (no args)     regenerate in dependency order, then verify every generator
#                 reports fresh (fail loud if any is still stale).
#   --check-only  skip regeneration; only run the freshness --check gates (the
#                 same set release-check runs) — a dry diagnosis of what is stale.
#
# Exit: 0 = all derived artifacts fresh; 1 = a generator still reports stale after
#       regeneration (silent no-op / hand-edited managed file) or a generator
#       errored; 2 = bad usage.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source-repo only — an installed downstream framework layer ships no generators.
if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  echo "regen-derived is for the Bubbles source repo, not an installed downstream framework layer." >&2
  exit 1
fi

# The release-manifest generator shells out to git; keep it non-interactive so a
# workspace with an SSH-passphrase-protected key never blocks the regen.
export GIT_TERMINAL_PROMPT=0
export GIT_SSH_COMMAND="${GIT_SSH_COMMAND:-ssh -o BatchMode=yes}"

CHECK_ONLY=0
case "${1:-}" in
  --check-only) CHECK_ONLY=1 ;;
  -h | --help)
    sed -n '2,33p' "$0"
    exit 0
    ;;
  "") ;;
  *)
    echo "regen-derived: unknown argument '$1' (expected --check-only, -h, or no args)." >&2
    exit 2
    ;;
esac

# Dependency-ordered generators. The interpreter for each matches release-check.sh
# (framework-stats is POSIX sh; the rest are bash). The release manifest MUST stay
# LAST — it checksums everything above it.
run_generator() {
  local interp="$1" script="$2" label="$3"
  echo "==> regenerating: $label"
  "$interp" "$SCRIPT_DIR/$script"
}

check_generator() {
  local interp="$1" script="$2" label="$3"
  echo "==> verifying fresh: $label"
  "$interp" "$SCRIPT_DIR/$script" --check
}

if [[ "$CHECK_ONLY" -eq 0 ]]; then
  run_generator sh generate-framework-stats.sh "framework stats (README / CHEATSHEET / html / framework-stats.*)"
  run_generator bash generate-cheatsheet.sh "cheatsheet"
  run_generator bash generate-capability-ledger-docs.sh "capability-ledger docs"
  run_generator bash generate-release-manifest.sh "release manifest (LAST — checksums everything above)"
  echo
fi

echo "Verifying derived-artifact freshness..."
fresh_failures=0
check_generator sh generate-framework-stats.sh "framework stats" || fresh_failures=$((fresh_failures + 1))
check_generator bash generate-cheatsheet.sh "cheatsheet" || fresh_failures=$((fresh_failures + 1))
check_generator bash generate-capability-ledger-docs.sh "capability-ledger docs" || fresh_failures=$((fresh_failures + 1))
check_generator bash generate-release-manifest.sh "release manifest" || fresh_failures=$((fresh_failures + 1))

if [[ "$fresh_failures" -gt 0 ]]; then
  echo "regen-derived: $fresh_failures derived artifact(s) still report stale after regeneration." >&2
  echo "A generator may have silently no-op'd, or a managed file was edited mid-run. Inspect the --check output above." >&2
  exit 1
fi

echo "regen-derived: all derived artifacts are fresh."
