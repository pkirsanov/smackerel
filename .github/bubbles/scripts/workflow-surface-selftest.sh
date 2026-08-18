#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
# v6.1 (S2 true split): mode definitions live in bubbles/workflows/modes.yaml.
# Mode-name checks read it; executionOptions (specReview, …) stay in
# workflows.yaml. Fall back to workflows.yaml for pre-split repos with an inline
# modes: block.
MODES_FILE="$ROOT_DIR/workflows/modes.yaml"
[[ -f "$MODES_FILE" ]] || MODES_FILE="$ROOT_DIR/workflows.yaml"

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

echo "Running workflow command-surface smoke test..."

check_pattern "$MODES_FILE" '^  full-delivery:$' "Workflow registry exposes full-delivery"
check_pattern "$ROOT_DIR/workflows.yaml" '^    specReview:$' "Workflow registry exposes the specReview execution option"
check_pattern "$SCRIPT_DIR/aliases.sh" '\[no-loose-ends\]="full-delivery"' "Sunnyvale alias resolves to full-delivery"
check_pattern "$ROOT_DIR/../agents/bubbles.workflow.agent.md" 'mode: .*full-delivery' "Workflow agent advertises full-delivery mode"
# IMP-049 SCOPE-6: the phase ID is the contract; its English title is not.
check_pattern "$ROOT_DIR/../agents/bubbles.workflow.agent.md" '^#+ .*Phase 0\.95' "Workflow agent documents a Phase 0.95 heading"
check_pattern "$ROOT_DIR/../agents/bubbles.workflow.agent.md" '^#+ .*Phase 0\.95.*full-delivery' "Phase 0.95 heading is scoped to full-delivery"
check_pattern "$ROOT_DIR/../agents/bubbles.super.agent.md" 'full-delivery' "Super agent knows about full-delivery"
check_pattern "$ROOT_DIR/../agents/bubbles.super.agent.md" 'no loose ends|until all green|release-candidate' "Super agent recognizes the lockdown request vocabulary"
check_pattern "$ROOT_DIR/../agents/bubbles.super.agent.md" 'specReview: once-before-implement|stale-spec check|Front-Door Policy' "Super agent exposes the one-shot spec review capability and front-door policy"
check_pattern "$ROOT_DIR/../agents/bubbles.super.agent.md" 'runtime lease|runtime doctor|shared Docker reuse' "Super agent exposes runtime coordination guidance"
# IMP-049 SCOPE-6. docs/CHEATSHEET.md and docs/its-not-rocket-appliances.html
# are GENERATED: generate-cheatsheet.sh renders both from
# bubbles/cheatsheet/modes.json and already owns their freshness. Asserting the
# rendered table cell or the rendered HTML card made a presentation detail gate
# a framework release. Assert the source of truth those cards are rendered FROM;
# the exact rendered markup moved to docs-wording-advisory.sh (advisory).
check_pattern "$ROOT_DIR/cheatsheet/modes.json" '"name": "full-delivery"' "Cheatsheet registry declares the full-delivery mode"
check_pattern "$ROOT_DIR/cheatsheet/modes.json" '"alias": "full-send"' "Cheatsheet registry declares the full-send alias"
check_optional_pattern "$ROOT_DIR/../docs/CHEATSHEET.md" 'bubbles runtime leases|bubbles runtime doctor|bubbles runtime summary' "Cheatsheet exposes runtime coordination commands"
check_optional_pattern "$ROOT_DIR/../docs/recipes/ask-the-super-first.md" 'full-delivery' "Super recipe demonstrates full-delivery guidance"
# The old pattern matched an example PROMPT SENTENCE the operator types. The
# contract is that the recipe demonstrates the runtime command family; the
# example's wording is not the contract.
check_optional_pattern "$ROOT_DIR/../docs/recipes/ask-the-super-first.md" '`bubbles runtime ' "Super recipe demonstrates runtime coordination commands"

if bash "$SCRIPT_DIR/workflow-registry-consistency.sh" --quiet; then
  pass "Workflow registry consistency check"
else
  fail "Workflow registry consistency check"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "workflow-surface selftest failed with $failures issue(s)."
  exit 1
fi

echo "workflow-surface selftest passed."