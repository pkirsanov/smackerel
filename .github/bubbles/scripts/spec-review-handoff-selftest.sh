#!/usr/bin/env bash
set -euo pipefail

# Selftest for SCOPE-13 spec-review MAJOR_DRIFT/OBSOLETE handoff routing.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
else
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

SPEC_REVIEW_AGENT="$REPO_ROOT/agents/bubbles.spec-review.agent.md"
WORKFLOW_AGENT="$REPO_ROOT/agents/bubbles.workflow.agent.md"
ORCHESTRATION_CORE="$REPO_ROOT/agents/bubbles_shared/workflow-orchestration-core.md"
INPUT_BOOTSTRAP="$REPO_ROOT/agents/bubbles_shared/workflow-input-bootstrap.md"
WORKFLOWS_FILE="$REPO_ROOT/bubbles/workflows.yaml"

failures=0
checks=0

pass() { checks=$((checks + 1)); echo "PASS: $*"; }
fail() { checks=$((checks + 1)); failures=$((failures + 1)); echo "FAIL: $*" >&2; }

display_path() {
  local path="$1"
  if [[ -n "${HOME:-}" && "$path" == "$HOME"/* ]]; then
    printf '~/%s' "${path#$HOME/}"
  else
    printf '%s' "$path"
  fi
}

require_file() {
  local path="$1"
  if [[ ! -f "$path" ]]; then
    fail "required file exists: $(display_path "$path")"
    return 1
  fi
  pass "required file exists: $(display_path "$path")"
}

assert_contains() {
  local label="$1"
  local path="$2"
  local pattern="$3"
  if grep -Eq "$pattern" "$path"; then
    pass "$label"
  else
    fail "$label"
    echo "  file: $(display_path "$path")" >&2
    echo "  missing regex: $pattern" >&2
  fi
}

assert_block_contains() {
  local label="$1"
  local path="$2"
  local start_pattern="$3"
  local required_pattern="$4"
  if awk -v start="$start_pattern" -v required="$required_pattern" '
    $0 ~ start { in_block=1 }
    in_block && $0 ~ required { found=1 }
    in_block && /^## / && $0 !~ start { in_block=0 }
    END { exit(found ? 0 : 1) }
  ' "$path"; then
    pass "$label"
  else
    fail "$label"
    echo "  file: $(display_path "$path")" >&2
    echo "  block regex: $start_pattern" >&2
    echo "  missing regex inside block: $required_pattern" >&2
  fi
}

echo "=== spec-review-handoff-selftest (SCOPE-13) ==="
echo "Repository: $(display_path "$REPO_ROOT")"
echo "Spec review agent: $(display_path "$SPEC_REVIEW_AGENT")"
echo "Workflow agent: $(display_path "$WORKFLOW_AGENT")"
echo "Orchestration core: $(display_path "$ORCHESTRATION_CORE")"
echo "Input bootstrap: $(display_path "$INPUT_BOOTSTRAP")"
echo "Workflows: $(display_path "$WORKFLOWS_FILE")"
echo ""

require_file "$SPEC_REVIEW_AGENT"
require_file "$WORKFLOW_AGENT"
require_file "$ORCHESTRATION_CORE"
require_file "$INPUT_BOOTSTRAP"
require_file "$WORKFLOWS_FILE"

echo ""
echo "--- S3/S4: spec-review handoff map dispatches severe done-spec drift ---"
assert_contains "spec-review mentions MAJOR_DRIFT" "$SPEC_REVIEW_AGENT" 'MAJOR_DRIFT'
assert_contains "spec-review mentions OBSOLETE" "$SPEC_REVIEW_AGENT" 'OBSOLETE'
assert_contains "spec-review handles done status" "$SPEC_REVIEW_AGENT" 'done(_with_concerns)?|done_with_concerns'
assert_contains "spec-review names bubbles.workflow dispatch" "$SPEC_REVIEW_AGENT" 'bubbles\.workflow'
assert_contains "spec-review names improve-existing mode" "$SPEC_REVIEW_AGENT" 'mode[=:][[:space:]]*improve-existing|mode=improve-existing|mode: improve-existing'
assert_block_contains "spec-review Phase 5 says invocation is mandatory" "$SPEC_REVIEW_AGENT" 'Phase 5' 'MANDATORY|MUST automatically invoke|MUST invoke'

echo ""
echo "--- S5: orchestration core honors dispatch automatically ---"
assert_contains "workflow auto-escalation has spec-review done-drift action" "$WORKFLOWS_FILE" 'specReview.*(Major|MAJOR|Obsolete|OBSOLETE|Drift|drift)'
assert_contains "workflow auto-escalation invokes improve-existing" "$WORKFLOWS_FILE" 'bubbles\.workflow.*mode=improve-existing|mode=improve-existing.*bubbles\.workflow|bubbles\.workflow.*mode: improve-existing|mode: improve-existing.*bubbles\.workflow'
assert_contains "orchestration core describes done spec-review route" "$ORCHESTRATION_CORE" 'spec-review.*(done|done_with_concerns).*(MAJOR_DRIFT|OBSOLETE)|MAJOR_DRIFT.*OBSOLETE.*improve-existing'
assert_contains "orchestration core requires automatic improve-existing dispatch" "$ORCHESTRATION_CORE" 'MUST.*(invoke|dispatch|parent-expand).*bubbles\.workflow.*improve-existing|improve-existing.*MUST.*(invoke|dispatch|parent-expand)'
assert_contains "workflow agent consumes improve-existing route packet" "$WORKFLOW_AGENT" 'spec-review.*improve-existing|improve-existing.*spec-review'
assert_contains "input bootstrap documents improve-existing stale route" "$INPUT_BOOTSTRAP" 'MAJOR_DRIFT.*OBSOLETE.*improve-existing|improve-existing.*MAJOR_DRIFT.*OBSOLETE'

echo ""
echo "=== Selftest verdict ==="
printf '  Checks: %d\n' "$checks"
printf '  Failures: %d\n' "$failures"

if [[ "$failures" -gt 0 ]]; then
  echo "spec-review-handoff-selftest: FAILED" >&2
  exit 1
fi

echo "spec-review-handoff-selftest: PASSED"
exit 0
