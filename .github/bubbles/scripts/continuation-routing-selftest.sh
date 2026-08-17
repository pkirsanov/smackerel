#!/usr/bin/env bash
# shellcheck disable=SC2016 # Regex patterns intentionally contain literal Markdown backticks.
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

check_absent() {
  local file_path="$1"
  local pattern="$2"
  local label="$3"

  if grep -Eq "$pattern" "$file_path"; then
    fail "$label"
  else
    pass "$label"
  fi
}

echo "Running continuation-routing regression selftest..."
echo "Scenario: stochastic-quality-sweep finishes a round, user says 'fix all found', workflow must preserve workflow-owned continuation."

check_pattern "$ROOT_DIR/../agents/bubbles.workflow.agent.md" 'fix all found|fix everything found|address rest|fix the rest' "Workflow agent recognizes continuation-shaped follow-up vocabulary"
check_pattern "$ROOT_DIR/../agents/bubbles.workflow.agent.md" 'CONTINUE → attempt one-mode workflow resume' "Workflow agent attempts one-mode workflow resume first"
check_pattern "$ROOT_DIR/../agents/bubbles.workflow.agent.md" 'Preserve an active granted mode such as `stochastic-quality-sweep`' "Workflow agent preserves granted stochastic sweep mode during resume"
check_pattern "$ROOT_DIR/../agents/bubbles.workflow.agent.md" 'preferredWorkflowMode: stochastic-quality-sweep' "Workflow agent emits workflow-owned continuation packets for stochastic sweeps"
check_pattern "$ROOT_DIR/../agents/bubbles.super.agent.md" 'fix all found from the last sweep' "Super agent documents the stochastic sweep continuation example"
check_pattern "$ROOT_DIR/../agents/bubbles.super.agent.md" 'Preserve `stochastic-quality-sweep`, `iterate`, and `full-delivery`' "Super agent continuation guard preserves active workflow modes"
check_pattern "$ROOT_DIR/../agents/bubbles_shared/workflow-delegation-core.md" 'completion checkpoint.*A candidate MUST NOT execute until the user explicitly requests that new work' "Context-free continue stops at the completion boundary"
check_pattern "$ROOT_DIR/../agents/bubbles_shared/workflow-delegation-core.md" 'continuation-intent-resolve\.sh' "Shared router uses the executable continuation classifier"
check_pattern "$ROOT_DIR/../agents/bubbles_shared/workflow-delegation-core.md" 'invokes? `bubbles\.recap`.*candidate-only next-priority work.*returns control to the user' "Context-free continue reports recap and candidate work without execution"
check_absent "$ROOT_DIR/../agents/bubbles_shared/workflow-delegation-core.md" 'route to `bubbles\.goal` or `bubbles\.iterate`' "Context-free continue no longer routes silently to goal or iterate"
for runner in bubbles.workflow bubbles.goal bubbles.iterate bubbles.sprint; do
  check_pattern "$ROOT_DIR/../agents/${runner}.agent.md" 'Terminal Recap Boundary' "$runner declares the terminal recap boundary"
  check_pattern "$ROOT_DIR/../agents/${runner}.agent.md" 'runSubagent\(bubbles\.recap\)' "$runner invokes recap before its final response"
done
check_absent "$ROOT_DIR/../agents/bubbles.goal.agent.md" 'runSubagent\(bubbles\.iterate\)' "Goal does not nest a workflow runner to obtain a candidate"
check_pattern "$ROOT_DIR/../agents/bubbles.workflow.agent.md" 'no non-terminal mode can be recovered.*runSubagent\(bubbles\.recap\).*stop' "Workflow stops at recap when completed continuation has no active mode"
check_pattern "$ROOT_DIR/../agents/bubbles.iterate.agent.md" 'If none exists, do not enter Scope Selection Logic' "Iterate does not select new work for completed continuation"
check_pattern "$ROOT_DIR/../agents/bubbles.iterate.agent.md" 'including targeted continuation language' "Iterate protects targeted continuation"
check_pattern "$ROOT_DIR/../agents/bubbles.goal.agent.md" 'continuation-intent-resolve\.sh' "Goal classifies targeted continuation before goal parsing"
check_pattern "$ROOT_DIR/../agents/bubbles.sprint.agent.md" 'continuation-intent-resolve\.sh' "Sprint classifies targeted continuation before queue parsing"
check_pattern "$ROOT_DIR/../agents/bubbles.iterate.agent.md" 'Refuse this priority when the raw input was classified `CONTINUE`' "Targeted terminal continuation cannot create a scope"
check_pattern "$ROOT_DIR/../agents/bubbles.super.agent.md" 'Never translate bare `continue`.*permission to run `/bubbles\.goal` or `/bubbles\.iterate` after completion' "Super does not convert completed continuation into new work"
check_pattern "$ROOT_DIR/../agents/bubbles.super.agent.md" 'Terminal Recap Boundary' "Super recaps completed stateful framework actions"
check_pattern "$ROOT_DIR/../agents/bubbles.recap.agent.md" 'next-priority candidate.*not a continuation target' "Recap keeps candidate work non-actionable"
check_pattern "$ROOT_DIR/../agents/bubbles.recap.agent.md" 'When dispatched with an inherited packet.*validate-packet.*do not run a second preflight' "Dispatched recap preserves the parent packet revision"
check_pattern "$ROOT_DIR/../agents/bubbles.recap.agent.md" 'repositoryRoot: <redacted-local-root>.*pathVisibility: redacted.*actionable: false.*target: none.*preferredWorkflowMode: none' "Terminal recap envelope uses a valid redacted projection"
check_pattern "$ROOT_DIR/../agents/bubbles.handoff.agent.md" 'repositoryRoot: <redacted-local-root>.*pathVisibility: redacted.*actionable: false.*target: none.*preferredWorkflowMode: none' "Terminal handoff envelope uses a valid redacted projection"
check_pattern "$ROOT_DIR/../docs/recipes/resume-work.md" 'tries to resume the active workflow context first' "Resume recipe documents active-workflow resume precedence"
check_pattern "$ROOT_DIR/../docs/recipes/resume-work.md" 'Recap may show one next-priority candidate as `not started`; it does not start that item' "Resume recipe documents terminal recap without execution"
# The framework README is a source-repo artifact. An installed downstream tree
# puts this selftest under .github/bubbles/scripts, so "$ROOT_DIR/.." is
# .github/ -- which never contains a README. Assert the claim where the file
# exists and say so explicitly everywhere else, rather than failing a
# downstream install for a document it was never given.
if [[ -f "$ROOT_DIR/../README.md" ]]; then
  check_pattern "$ROOT_DIR/../README.md" 'returns a completion recap and an unstarted next-priority candidate' "README documents the completed-state boundary"
else
  echo "SKIP: README documents the completed-state boundary (no README alongside the framework root)"
fi
check_pattern "$ROOT_DIR/scripts/aliases.sh" '\[keep-going\]="resume-only"' "Keep-going alias resolves to resume-only"
check_pattern "$ROOT_DIR/scripts/aliases.sh" '\[pick-next\]="iterate"' "Pick-next alias explicitly resolves to iterate"
check_pattern "$ROOT_DIR/scripts/aliases.sh" '\[next-on-the-board\]="bubbles\.iterate"' "Next-on-the-board explicitly selects new work"
next_alias_output="$(bash "$ROOT_DIR/scripts/cli.sh" sunnyvale next-on-the-board)"
if grep -qF 'next-on-the-board → bubbles.iterate' <<< "$next_alias_output"; then
  pass "Generated next-on-the-board alias matches runtime"
else
  fail "Generated next-on-the-board alias matches runtime"
fi
keep_going_output="$(bash "$ROOT_DIR/scripts/cli.sh" sunnyvale keep-going)"
if grep -qF 'keep-going → workflow mode: resume-only' <<< "$keep_going_output"; then
  pass "Keep-going remains a resume request at runtime"
else
  fail "Keep-going remains a resume request at runtime"
fi
if [[ "$(bash "$ROOT_DIR/scripts/continuation-intent-resolve.sh")" == "OTHER" ]]; then
  pass "Bare iterate input remains explicit new-work entrypoint input"
else
  fail "Bare iterate input remains explicit new-work entrypoint input"
fi
if bash "$ROOT_DIR/scripts/is-terminal-for-mode.sh" in_progress resume-only >/dev/null 2>&1; then
  fail "Transient resume does not treat in_progress as terminal"
else
  pass "Transient resume does not treat in_progress as terminal"
fi
if bash "$ROOT_DIR/scripts/is-terminal-for-mode.sh" validated validate-to-doc >/dev/null 2>&1; then
  pass "Recovered mode retains its own terminal semantics"
else
  fail "Recovered mode retains its own terminal semantics"
fi
check_pattern "$ROOT_DIR/../docs/recipes/quality-sweep.md" 'fix all found|address the rest' "Quality sweep recipe documents workflow-owned continuation language"
check_pattern "$ROOT_DIR/../docs/guides/WORKFLOW_MODES.md" 'Continuation-shaped input.*preserve the active workflow mode' "Workflow modes guide documents continuation resume precedence"

if [[ "$failures" -gt 0 ]]; then
  echo "continuation-routing selftest failed with $failures issue(s)."
  exit 1
fi

echo "continuation-routing selftest passed."