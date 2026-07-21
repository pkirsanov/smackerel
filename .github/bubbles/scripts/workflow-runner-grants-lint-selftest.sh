#!/usr/bin/env bash
# Hermetic adversarial selftest for workflow-runner-grants-lint.sh.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
LINT="$SCRIPT_DIR/workflow-runner-grants-lint.sh"

if ! command -v yq >/dev/null 2>&1; then
  echo "workflow-runner-grants-lint-selftest: SKIP (yq v4 not installed)"
  exit 0
fi

selftest_tmp_base="${TMPDIR:-$HOME/.cache}"
mkdir -p "$selftest_tmp_base"
TMPDIR="$(mktemp -d "$selftest_tmp_base/bubbles-workflow-runner-grants.XXXXXX")"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

failures=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1" >&2; failures=$((failures + 1)); }

fresh_fixture() {
  local destination="$1"
  rm -rf "$destination"
  mkdir -p "$destination/bubbles/workflows" "$destination/bubbles/scripts" "$destination/agents"
  cp "$REPO_ROOT/bubbles/agent-capabilities.yaml" "$destination/bubbles/agent-capabilities.yaml"
  cp "$REPO_ROOT/bubbles/workflows.yaml" "$destination/bubbles/workflows.yaml"
  cp "$REPO_ROOT/bubbles/workflows/modes.yaml" "$destination/bubbles/workflows/modes.yaml"
  cp "$REPO_ROOT/bubbles/intent-routes.yaml" "$destination/bubbles/intent-routes.yaml"
  cp "$LINT" "$destination/bubbles/scripts/workflow-runner-grants-lint.sh"
  cp "$REPO_ROOT"/agents/bubbles.*.agent.md "$destination/agents/"
}

run_case() {
  local label="$1"
  local root="$2"
  local expected="$3"
  local marker="$4"
  local log="$TMPDIR/${label}.log"
  local exit_code=0

  set +e
  bash "$root/bubbles/scripts/workflow-runner-grants-lint.sh" --repo-root "$root" >"$log" 2>&1
  exit_code=$?
  set -e

  if [[ "$exit_code" -eq "$expected" ]]; then
    pass "$label exit=$expected"
  else
    fail "$label expected exit=$expected got $exit_code"
  fi
  if grep -Fq "$marker" "$log"; then
    pass "$label emitted marker '$marker'"
  else
    fail "$label missing marker '$marker'"
  fi
}

fixture="$TMPDIR/repo"
fresh_fixture "$fixture"
run_case clean "$fixture" 0 "workflow-runner-grants-lint: PASS"

fresh_fixture "$fixture"
yq -i '.workflowModeGrants.agents."bubbles.bug".modes += ["not-a-real-mode"]' "$fixture/bubbles/agent-capabilities.yaml"
run_case unknown-mode "$fixture" 1 "references unknown mode 'not-a-real-mode'"

fresh_fixture "$fixture"
yq -i 'del(.workflowModeGrants.agents."bubbles.bug")' "$fixture/bubbles/agent-capabilities.yaml"
run_case missing-grant "$fixture" 1 "enables workflow execution without a grant"

fresh_fixture "$fixture"
yq -i '.agents."bubbles.releases".class = "execution-owner"' "$fixture/bubbles/agent-capabilities.yaml"
run_case non-orchestrator "$fixture" 1 "must have class orchestrator"

fresh_fixture "$fixture"
yq -i '.routes[0].targetAgent = "bubbles.validate"' "$fixture/bubbles/intent-routes.yaml"
run_case ungranted-intent-route "$fixture" 1 "intent route targets 'bubbles.validate' for ungranted mode"

fresh_fixture "$fixture"
awk '
  /## Outcome-First Dispatch Contract/ && !inserted {
    print "preferred: runSubagent(bubbles.workflow): nested execution"
    inserted=1
  }
  { print }
' "$fixture/agents/bubbles.goal.agent.md" > "$TMPDIR/goal-mutated.md"
mv "$TMPDIR/goal-mutated.md" "$fixture/agents/bubbles.goal.agent.md"
run_case nested-runner "$fixture" 1 "nested workflow-runner dispatch found"

if [[ "$failures" -gt 0 ]]; then
  echo "workflow-runner-grants-lint-selftest: FAIL ($failures assertion(s))" >&2
  exit 1
fi

echo "workflow-runner-grants-lint-selftest: PASS"