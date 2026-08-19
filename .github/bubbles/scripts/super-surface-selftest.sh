#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

failures=0

pass() {
  echo "PASS: $1"
}

fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

check_pattern() {
  local file_path="$1"
  local pattern="$2"
  local label="$3"

  if grep -Eq "$pattern" "$file_path"; then
    pass "$label"
  else
    fail "$label"
  fi
}

check_optional_pattern() {
  local file_path="$1"
  local pattern="$2"
  local label="$3"

  if [[ ! -f "$file_path" ]]; then
    echo "SKIP: $label (missing $(basename "$file_path"))"
    return 0
  fi

  check_pattern "$file_path" "$pattern" "$label"
}

# IMP-049 SCOPE-6. A doc that enumerates surfaces makes ONE claim -- every named
# surface is advertised. Pinning the rendered serial-comma sentence made that
# claim break on reordering, on repunctuation, and on the word "and". Require
# each named surface independently, in any order and any sentence shape; the
# exact sentence moved to docs-wording-advisory.sh (advisory).
check_tokens() {
  local file_path="$1"
  local label="$2"
  shift 2
  local missing=()
  local token

  if [[ ! -f "$file_path" ]]; then
    fail "$label (missing $(basename "$file_path"))"
    return 0
  fi
  for token in "$@"; do
    grep -Fq -- "$token" "$file_path" || missing+=("$token")
  done
  if [[ "${#missing[@]}" -eq 0 ]]; then
    pass "$label"
  else
    fail "$label (missing surface token(s): ${missing[*]})"
  fi
}

check_optional_tokens() {
  local file_path="$1"
  local label="$2"
  shift 2

  if [[ ! -f "$file_path" ]]; then
    echo "SKIP: $label (missing $(basename "$file_path"))"
    return 0
  fi
  check_tokens "$file_path" "$label" "$@"
}

echo "Running super-surface awareness selftest..."

SUPER_AGENT="$ROOT_DIR/../agents/bubbles.super.agent.md"
SUPER_PROMPT="$ROOT_DIR/../prompts/bubbles.super.prompt.md"
AGENT_MANUAL="$ROOT_DIR/../docs/guides/AGENT_MANUAL.md"
SUPER_RECIPE="$ROOT_DIR/../docs/recipes/ask-the-super-first.md"

check_pattern "$SUPER_AGENT" 'Source repo: `ls agents/bubbles\.\*\.agent\.md`; downstream repo: `ls \.github/agents/bubbles\.\*\.agent\.md`' "Super agent discovers source and downstream agent inventories"
check_pattern "$SUPER_AGENT" 'source repo `bubbles/workflows/modes\.yaml`, downstream repo `\.github/bubbles/workflows/modes\.yaml`' "Super agent discovers workflow registry by repo posture"
check_pattern "$SUPER_AGENT" 'Source repo: `ls skills/\*/SKILL\.md`; downstream repo: `ls \.github/skills/\*/SKILL\.md`' "Super agent discovers source and downstream skills"
check_pattern "$SUPER_AGENT" 'Source repo: `ls instructions/\*\.md`; downstream repo: `ls \.github/instructions/\*\.instructions\.md`' "Super agent discovers source and downstream instructions"
check_pattern "$SUPER_AGENT" 'docs/CATALOG\.md' "Super agent uses the recipe catalog as a feature map"
check_pattern "$SUPER_AGENT" 'framework-events|run-state|repo-readiness|action-risk-registry\.yaml' "Super agent knows the new control-plane command surfaces"
check_pattern "$SUPER_AGENT" 'Feature Coverage Guard' "Super agent documents the broad capability coverage guard"
# IMP-049 SCOPE-6: the CLI path is the interface; the Markdown table cell that
# frames it is presentation. The rendered rows moved to docs-wording-advisory.sh.
check_pattern "$SUPER_AGENT" '`bash bubbles/scripts/cli\.sh' "Super agent documents source-repo CLI path resolution"
check_pattern "$SUPER_AGENT" '`bash \.github/bubbles/scripts/cli\.sh' "Super agent documents downstream CLI path resolution"
check_tokens "$SUPER_PROMPT" "Super prompt advertises expanded framework ops scope" \
  'framework validation' 'release hygiene' 'run-state' 'repo-readiness'
check_tokens "$SUPER_PROMPT" "Super prompt requires live-surface discovery" \
  'agents' 'workflow modes' 'recipes' 'skills' 'instructions' 'CLI commands' \
  'run-state' 'framework events' 'risk classes'
check_optional_tokens "$AGENT_MANUAL" "Agent manual documents super surface discovery breadth" \
  'recipe' 'skill' 'instruction' 'risk' 'runtime'
check_optional_pattern "$SUPER_RECIPE" '`bash bubbles/scripts/cli\.sh' "Super recipe documents source-repo CLI resolution"
check_optional_pattern "$SUPER_RECIPE" '`bash \.github/bubbles/scripts/cli\.sh' "Super recipe documents downstream CLI resolution"

if [[ "$failures" -gt 0 ]]; then
  echo "super-surface selftest failed with $failures issue(s)."
  exit 1
fi

echo "super-surface selftest passed."