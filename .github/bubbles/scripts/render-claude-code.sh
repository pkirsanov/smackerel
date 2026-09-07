#!/usr/bin/env bash
set -euo pipefail

# render-claude-code.sh
#
# Renders Bubbles' Copilot-format agents/prompts/skills/instructions into
# Claude Code's native subagent (.claude/agents/) + slash-command
# (.claude/commands/) format, plus a portable copy of skills/instructions
# under .claude/skills/ and .claude/instructions/ so relative links in
# rendered agent bodies resolve.
#
# This is a second OUTPUT target alongside the existing Copilot install
# (.github/agents, .github/prompts, .github/skills). It never touches
# .github/ or any source file — install.sh calls this right after its
# existing agents/prompts/skills copy steps.
#
# Usage:
#   bash bubbles/scripts/render-claude-code.sh --source <bubbles-checkout> --dest <downstream-repo-root>
#
# Dependencies: python3 (stdlib only), yq (mikefarah v4+, already an accepted
# soft dependency elsewhere in this repo, e.g. adversarial-resolve.sh).

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if ! command -v yq >/dev/null 2>&1; then
  echo "render-claude-code: yq (mikefarah v4+) not found — skipping Claude Code render (Copilot output is unaffected)" >&2
  exit 0
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "render-claude-code: python3 not found — skipping Claude Code render (Copilot output is unaffected)" >&2
  exit 0
fi

exec python3 "$SCRIPT_DIR/render_claude_code.py" "$@"
