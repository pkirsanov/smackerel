#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
WORKFLOWS_FILE="$REPO_ROOT/bubbles/workflows.yaml"
# v6.1 (S2 true split): mode definitions live in bubbles/workflows/modes.yaml.
# mode_inventory() parses them from there unless workflows.yaml still embeds an
# inline modes: block (pre-split / fixtures).
MODES_FILE="$REPO_ROOT/bubbles/workflows/modes.yaml"
if grep -qE '^modes:' "$WORKFLOWS_FILE" 2>/dev/null || [[ ! -f "$MODES_FILE" ]]; then
  MODES_FILE="$WORKFLOWS_FILE"
fi
WORKFLOW_AGENT_FILE="$REPO_ROOT/agents/bubbles.workflow.agent.md"
CHEATSHEET_FILE="$REPO_ROOT/docs/CHEATSHEET.md"
STATS_FILE="$REPO_ROOT/docs/generated/framework-stats.json"
CLI_FILE="$REPO_ROOT/bubbles/scripts/cli.sh"

quiet=false
if [[ "${1:-}" == "--quiet" ]]; then
  quiet=true
fi

fail() {
  if [[ "$quiet" == "false" ]]; then
    echo "FAIL: $1"
  fi
  exit 1
}

assert_file() {
  local path="$1"
  local label="$2"

  [[ -f "$path" ]] || fail "$label missing: $path"
}

mode_inventory() {
  # Only collect 2-indent keys that live under the top-level `modes:` section.
  # Top-level keys match `^[a-zA-Z]` (column 0). Any new top-level key that is
  # NOT `modes:` flips us out of the modes section so unrelated 2-indent keys
  # under `outcomeStates:`, `modeTemplates:`, `phases:`, etc. are excluded.
  # The `description:`-as-next-line heuristic additionally filters out config
  # blocks like `phaseRelevance:` that live inside `modes:` but are not modes.
  awk '
    BEGIN { in_modes = 0 }
    /^[a-zA-Z][a-zA-Z0-9_-]*:/ {
      in_modes = ($0 ~ /^modes:/) ? 1 : 0
      next
    }
    in_modes && /^  [a-z][a-z0-9-]*:$/ {
      mode = $1
      sub(/:$/, "", mode)
      if ((getline next_line) > 0) {
        if (next_line ~ /^    description:/) {
          print mode
        }
      }
    }
  ' "$MODES_FILE"
}

supported_options_inventory() {
  grep -m1 '^\- `mode: ' "$WORKFLOW_AGENT_FILE" \
    | sed -E 's/^\- `mode: ([^`]+)`$/\1/' \
    | tr '|' '\n' \
    | sed '/^$/d'
}

assert_file "$WORKFLOWS_FILE" "Workflow registry"
assert_file "$WORKFLOW_AGENT_FILE" "Workflow agent"
assert_file "$CLI_FILE" "CLI"

actual_modes="$(mode_inventory | sort)"
agent_modes="$(supported_options_inventory | sort)"

[[ -n "$actual_modes" ]] || fail "No delivery modes discovered in workflows.yaml"
[[ -n "$agent_modes" ]] || fail "No supported mode inventory discovered in bubbles.workflow.agent.md"

if [[ "$actual_modes" != "$agent_modes" ]]; then
  if [[ "$quiet" == "false" ]]; then
    echo "Workflow mode registry mismatch"
    echo "Expected from workflows.yaml:"
    printf '%s\n' "$actual_modes"
    echo "Advertised by workflow agent:"
    printf '%s\n' "$agent_modes"
  fi
  exit 1
fi

if [[ -f "$CHEATSHEET_FILE" ]]; then
  grep -q 'bubbles skill-proposals' "$CHEATSHEET_FILE" || fail "Cheatsheet missing skill-proposals command"
  grep -q 'bubbles profile' "$CHEATSHEET_FILE" || fail "Cheatsheet missing profile command"
  grep -q 'bubbles runtime leases' "$CHEATSHEET_FILE" || fail "Cheatsheet missing runtime leases command"
  grep -q 'bubbles runtime doctor' "$CHEATSHEET_FILE" || fail "Cheatsheet missing runtime doctor command"
  grep -q 'bubbles runtime summary' "$CHEATSHEET_FILE" || fail "Cheatsheet missing runtime summary command"
fi
grep -q 'skill-proposals' "$CLI_FILE" || fail "CLI missing skill-proposals command surface"
grep -q 'profile' "$CLI_FILE" || fail "CLI missing profile command surface"

if [[ -f "$STATS_FILE" ]]; then
  stats_modes="$(grep -oE '"workflowModes":[[:space:]]*[0-9]+' "$STATS_FILE" | sed -E 's/.*:[[:space:]]*([0-9]+)/\1/' || true)"
  actual_mode_count="$(printf '%s\n' "$actual_modes" | sed '/^$/d' | wc -l | tr -d ' ')"
  [[ -n "$stats_modes" ]] || fail "Generated stats file missing workflowModes count"
  [[ "$stats_modes" == "$actual_mode_count" ]] || fail "Generated stats workflowModes count ($stats_modes) does not match registry ($actual_mode_count)"
fi

if [[ "$quiet" == "false" ]]; then
  echo "workflow-registry consistency check passed."
fi