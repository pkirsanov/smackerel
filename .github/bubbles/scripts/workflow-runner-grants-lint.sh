#!/usr/bin/env bash
# Enforce Gate G064: workflow modes run only in an authorized top-level runner.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEFAULT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
REPO_ROOT="$DEFAULT_ROOT"

if [[ "${1:-}" == "--repo-root" ]]; then
  shift
  REPO_ROOT="${1:?--repo-root requires a path}"
  shift
fi
if [[ $# -gt 0 ]]; then
  echo "workflow-runner-grants-lint: unknown argument: $1" >&2
  exit 2
fi

if [[ -f "$REPO_ROOT/bubbles/agent-capabilities.yaml" ]]; then
  BUBBLES_DIR="$REPO_ROOT/bubbles"
  AGENTS_DIR="$REPO_ROOT/agents"
else
  BUBBLES_DIR="$REPO_ROOT/.github/bubbles"
  AGENTS_DIR="$REPO_ROOT/.github/agents"
fi

CAPABILITIES="$BUBBLES_DIR/agent-capabilities.yaml"
WORKFLOWS="$BUBBLES_DIR/workflows.yaml"
MODES="$BUBBLES_DIR/workflows/modes.yaml"
INTENT_ROUTES="$BUBBLES_DIR/intent-routes.yaml"

for required in "$CAPABILITIES" "$WORKFLOWS" "$MODES"; do
  if [[ ! -f "$required" ]]; then
    echo "workflow-runner-grants-lint: required file missing: $required" >&2
    exit 2
  fi
done

if ! command -v yq >/dev/null 2>&1; then
  echo "workflow-runner-grants-lint: yq v4 is required (Gate G064 fails closed)" >&2
  exit 1
fi

failures=0
fail() {
  echo "G064 WORKFLOW_RUNNER_GRANT_VIOLATION: $1" >&2
  failures=$((failures + 1))
}

policy_model="$(yq -r '.workflowExecutionPolicy.defaultExecutionModel // ""' "$WORKFLOWS")"
policy_default="$(yq -r '.workflowExecutionPolicy.defaultAllowed | tostring' "$WORKFLOWS")"
policy_top_level="$(yq -r '.workflowExecutionPolicy.topLevelRuntimeRequired | tostring' "$WORKFLOWS")"
policy_nested="$(yq -r '.workflowExecutionPolicy.nestedWorkflowRunnerDispatch // ""' "$WORKFLOWS")"
policy_owner="$(yq -r '.workflowExecutionPolicy.controlPhaseOwner // ""' "$WORKFLOWS")"

[[ "$policy_model" == "direct-authorized-runner" ]] || fail "defaultExecutionModel must be direct-authorized-runner"
[[ "$policy_default" == "false" ]] || fail "defaultAllowed must be false"
[[ "$policy_top_level" == "true" ]] || fail "topLevelRuntimeRequired must be true"
[[ "$policy_nested" == "forbidden" ]] || fail "nestedWorkflowRunnerDispatch must be forbidden"
[[ "$policy_owner" == "activeWorkflowRunner" ]] || fail "controlPhaseOwner must be activeWorkflowRunner"

for phase in analyze discover bootstrap finalize; do
  owner="$(yq -r ".phases.${phase}.owner // \"\"" "$WORKFLOWS")"
  [[ "$owner" == "activeWorkflowRunner" ]] || fail "phase '$phase' must be owned by activeWorkflowRunner"
done

mode_exists() {
  local mode="$1"
  yq -e ".modes.\"${mode}\" != null" "$MODES" >/dev/null 2>&1
}

grant_exists() {
  local agent="$1"
  yq -e ".workflowModeGrants.agents.\"${agent}\" != null" "$CAPABILITIES" >/dev/null 2>&1
}

runner_allows_mode() {
  local agent="$1"
  local mode="$2"
  grant_exists "$agent" || return 1
  if yq -e ".workflowModeGrants.agents.\"${agent}\".excludedModes[] | select(. == \"${mode}\")" "$CAPABILITIES" >/dev/null 2>&1; then
    return 1
  fi
  yq -e ".workflowModeGrants.agents.\"${agent}\".modes[] | select(. == \"*\" or . == \"${mode}\")" "$CAPABILITIES" >/dev/null 2>&1
}

while IFS= read -r runner; do
  [[ -n "$runner" ]] || continue

  if ! yq -e ".agents.\"${runner}\" != null" "$CAPABILITIES" >/dev/null 2>&1; then
    fail "granted runner '$runner' is absent from agents"
    continue
  fi

  runner_class="$(yq -r ".agents.\"${runner}\".class // \"\"" "$CAPABILITIES")"
  runner_enabled="$(yq -r ".agents.\"${runner}\".canExecuteWorkflowModes // false" "$CAPABILITIES")"
  [[ "$runner_class" == "orchestrator" ]] || fail "granted runner '$runner' must have class orchestrator"
  [[ "$runner_enabled" == "true" ]] || fail "granted runner '$runner' must set canExecuteWorkflowModes: true"

  runner_file="$AGENTS_DIR/${runner}.agent.md"
  [[ -f "$runner_file" ]] || fail "granted runner '$runner' has no agent file at $runner_file"

  while IFS= read -r mode; do
    [[ -n "$mode" || "$mode" == "*" ]] || continue
    [[ "$mode" == "*" ]] && continue
    mode_exists "$mode" || fail "runner '$runner' references unknown mode '$mode'"
  done < <(yq -r ".workflowModeGrants.agents.\"${runner}\".modes[]" "$CAPABILITIES" 2>/dev/null || true)

  while IFS= read -r excluded; do
    [[ -n "$excluded" ]] || continue
    mode_exists "$excluded" || fail "runner '$runner' excludes unknown mode '$excluded'"
  done < <(yq -r ".workflowModeGrants.agents.\"${runner}\".excludedModes[]" "$CAPABILITIES" 2>/dev/null || true)
done < <(yq -r '.workflowModeGrants.agents | keys | .[]' "$CAPABILITIES")

while IFS= read -r enabled_runner; do
  [[ -n "$enabled_runner" ]] || continue
  grant_exists "$enabled_runner" || fail "agent '$enabled_runner' enables workflow execution without a grant"
done < <(yq -r '.agents | to_entries | .[] | select(.value.canExecuteWorkflowModes == true) | .key' "$CAPABILITIES")

while IFS=$'\t' read -r meta_mode meta_owner; do
  [[ -n "$meta_mode" && -n "$meta_owner" ]] || continue
  mode_exists "$meta_mode" || fail "meta mode '$meta_mode' does not exist"
  grant_exists "$meta_owner" || {
    fail "meta mode owner '$meta_owner' has no workflow grant"
    continue
  }
  if ! runner_allows_mode "$meta_owner" "$meta_mode"; then
    fail "meta mode owner '$meta_owner' is not granted '$meta_mode'"
  fi
done < <(yq -r '.workflowExecutionPolicy.metaModeOwners | to_entries | .[] | [.key, .value] | @tsv' "$WORKFLOWS")

if [[ -f "$INTENT_ROUTES" ]]; then
  while IFS=$'\t' read -r route_agent route_mode; do
    [[ -n "$route_agent" && -n "$route_mode" ]] || continue
    mode_exists "$route_mode" || {
      fail "intent route references unknown mode '$route_mode'"
      continue
    }
    runner_allows_mode "$route_agent" "$route_mode" || fail "intent route targets '$route_agent' for ungranted mode '$route_mode'"
  done < <(yq -r '.routes[] | select(.targetMode != null) | [.targetAgent, .targetMode] | @tsv' "$INTENT_ROUTES")
fi

workflow_root_limit="$(yq -r '.workflowModeGrants.agents."bubbles.workflow".maxRootModesPerRun // 0' "$CAPABILITIES")"
[[ "$workflow_root_limit" == "1" ]] || fail "bubbles.workflow maxRootModesPerRun must be 1"

for runner_file in \
  "$AGENTS_DIR/bubbles.goal.agent.md" \
  "$AGENTS_DIR/bubbles.sprint.agent.md" \
  "$AGENTS_DIR/bubbles.iterate.agent.md" \
  "$AGENTS_DIR/bubbles.bug.agent.md" \
  "$AGENTS_DIR/bubbles.releases.agent.md" \
  "$AGENTS_DIR/bubbles.train.agent.md" \
  "$AGENTS_DIR/bubbles.upkeep.agent.md" \
  "$AGENTS_DIR/bubbles.propagate.agent.md" \
  "$AGENTS_DIR/bubbles.stabilize.agent.md" \
  "$AGENTS_DIR/bubbles.retro.agent.md" \
  "$AGENTS_DIR/bubbles.journey.agent.md"; do
  [[ -f "$runner_file" ]] || continue
  if grep -nE 'preferred:[[:space:]]*runSubagent\(bubbles\.(workflow|goal|sprint|iterate|bug|releases|train|upkeep|propagate)\)|call_runSubagent:.*runSubagent\(bubbles\.(workflow|goal|sprint|iterate|bug|releases|train|upkeep|propagate)\)' "$runner_file"; then
    fail "nested workflow-runner dispatch found in $(basename "$runner_file")"
  fi
done

if [[ "$failures" -gt 0 ]]; then
  echo "workflow-runner-grants-lint: FAIL ($failures violation(s))" >&2
  exit 1
fi

runner_count="$(yq -r '.workflowModeGrants.agents | length' "$CAPABILITIES")"
echo "workflow-runner-grants-lint: PASS ($runner_count authorized runners, default deny, direct execution)"