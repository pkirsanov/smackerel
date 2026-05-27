#!/usr/bin/env bash
# bubbles/scripts/is-terminal-for-mode.sh
#
# Returns 0 (true) when a given status is the terminal status for a workflow
# mode, 1 (false) otherwise.
#
# A status is "terminal-for-mode" when ANY of these are true:
#   1. status == "done"           (universal terminal status)
#   2. status == <mode>.statusCeiling
#   3. status appears in <mode>.terminalAliases[]
#
# Rationale: workflow modes like `validate-to-doc`, `docs-only`, and
# `chaos-hardening` have terminal statuses other than `done` (validated,
# docs_updated, delivered_pending_activation, etc.). Portfolio reports and
# status sweeps MUST treat those as completed-for-mode, not as open work.
# Promotion past the mode's ceiling is forbidden by state-transition-guard.sh
# and is NOT a backlog item.
#
# Usage:
#   bash bubbles/scripts/is-terminal-for-mode.sh <status> <mode>
#
# Exit codes:
#   0 = status is terminal-for-mode
#   1 = status is NOT terminal-for-mode
#   2 = error (missing args, unknown mode, yq missing, malformed workflows.yaml)
#
# Examples:
#   bash bubbles/scripts/is-terminal-for-mode.sh done full-delivery        # 0
#   bash bubbles/scripts/is-terminal-for-mode.sh validated validate-to-doc # 0
#   bash bubbles/scripts/is-terminal-for-mode.sh docs_updated docs-only    # 0
#   bash bubbles/scripts/is-terminal-for-mode.sh in_progress full-delivery # 1
#   bash bubbles/scripts/is-terminal-for-mode.sh validated full-delivery   # 1

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
WORKFLOWS_FILE="${BUBBLES_WORKFLOWS_FILE:-$ROOT_DIR/bubbles/workflows.yaml}"

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/is-terminal-for-mode.sh <status> <mode>

Exit codes:
  0 = status is terminal-for-mode
  1 = status is NOT terminal-for-mode
  2 = error (missing args, unknown mode, yq missing, malformed workflows.yaml)
EOF
}

if [[ $# -ne 2 ]]; then
  usage >&2
  exit 2
fi

STATUS="$1"
MODE="$2"

if [[ -z "$STATUS" || -z "$MODE" ]]; then
  echo "is-terminal-for-mode: status and mode must be non-empty" >&2
  exit 2
fi

# Universal terminal status: `done` is always terminal regardless of mode.
if [[ "$STATUS" == "done" ]]; then
  exit 0
fi

if ! command -v yq >/dev/null 2>&1; then
  echo "is-terminal-for-mode: yq (mikefarah, v4+) is required" >&2
  exit 2
fi

if [[ ! -f "$WORKFLOWS_FILE" ]]; then
  echo "is-terminal-for-mode: workflows file not found: $WORKFLOWS_FILE" >&2
  exit 2
fi

# Resolve the mode (honoring `inherits:` template chains) so that ceilings
# inherited from base-delivery / delivery-* templates are visible. Falls back
# to the raw mode entry if mode-resolver.sh is missing.
resolved=""
if [[ -x "$SCRIPT_DIR/mode-resolver.sh" ]]; then
  resolved="$(bash "$SCRIPT_DIR/mode-resolver.sh" "$MODE" 2>/dev/null || true)"
fi

if [[ -n "$resolved" ]]; then
  ceiling="$(printf '%s\n' "$resolved" | yq -r '.statusCeiling // ""' 2>/dev/null || true)"
  aliases="$(printf '%s\n' "$resolved" | yq -r '.terminalAliases[]? // empty' 2>/dev/null || true)"
else
  ceiling="$(yq -r ".modes[\"$MODE\"].statusCeiling // \"\"" "$WORKFLOWS_FILE" 2>/dev/null || true)"
  aliases="$(yq -r ".modes[\"$MODE\"].terminalAliases[]? // empty" "$WORKFLOWS_FILE" 2>/dev/null || true)"
fi

if [[ -z "$ceiling" || "$ceiling" == "null" ]]; then
  echo "is-terminal-for-mode: unknown mode or missing statusCeiling: $MODE" >&2
  exit 2
fi

if [[ "$STATUS" == "$ceiling" ]]; then
  exit 0
fi

if [[ -n "$aliases" ]]; then
  while IFS= read -r alias; do
    [[ -z "$alias" ]] && continue
    if [[ "$STATUS" == "$alias" ]]; then
      exit 0
    fi
  done <<< "$aliases"
fi

exit 1
