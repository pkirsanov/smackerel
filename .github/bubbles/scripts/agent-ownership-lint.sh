#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ "$script_dir" == *"/.github/bubbles/scripts" ]]; then
  root_dir="${script_dir%/.github/bubbles/scripts}"
  agents_dir="$root_dir/.github/agents"
  shared_dir="$agents_dir/bubbles_shared"
  workflows_file="$root_dir/.github/bubbles/workflows.yaml"
  ownership_file="$root_dir/.github/bubbles/agent-ownership.yaml"
  capabilities_file="$root_dir/.github/bubbles/agent-capabilities.yaml"
else
  root_dir="${script_dir%/bubbles/scripts}"
  agents_dir="$root_dir/agents"
  shared_dir="$agents_dir/bubbles_shared"
  workflows_file="$root_dir/bubbles/workflows.yaml"
  ownership_file="$root_dir/bubbles/agent-ownership.yaml"
  capabilities_file="$root_dir/bubbles/agent-capabilities.yaml"
fi

errors=0

check_no_match() {
  local file="$1"
  local pattern="$2"
  local message="$3"
  if grep -nE "$pattern" "$file"; then
    echo "ERROR: $message"
    errors=1
  fi
}

check_has_match() {
  local file="$1"
  local pattern="$2"
  local message="$3"
  if ! grep -nE "$pattern" "$file" >/dev/null; then
    echo "ERROR: $message"
    errors=1
  fi
}

check_has_match "$ownership_file" '^version:' 'agent ownership manifest missing version header'
check_has_match "$capabilities_file" '^version:' 'agent capabilities manifest missing version header'
check_has_match "$capabilities_file" '^childWorkflowPolicy:' 'agent capabilities manifest missing child workflow policy block'
check_has_match "$capabilities_file" '^resultPolicy:' 'agent capabilities manifest missing result policy block'
check_has_match "$shared_dir/agent-common.md" '^## Artifact Ownership And Delegation Contract$' 'agent-common.md missing ownership contract section'
if grep -nE 'name: artifact_ownership_enforcement_gate' "$workflows_file" >/dev/null; then
  :
else
  check_has_match "$workflows_file" 'name: agent_ownership_gate' 'workflows.yaml missing ownership enforcement gate (expected consolidated artifact_ownership_enforcement_gate or legacy agent_ownership_gate)'
  check_has_match "$workflows_file" 'name: capability_delegation_gate' 'workflows.yaml missing legacy capability delegation gate when consolidated artifact_ownership_enforcement_gate is absent'
  check_has_match "$workflows_file" 'name: owner_only_remediation_gate' 'workflows.yaml missing legacy owner-only remediation gate when consolidated artifact_ownership_enforcement_gate is absent'
fi
check_has_match "$workflows_file" 'name: concrete_result_gate' 'workflows.yaml missing G063 concrete result gate'
check_has_match "$workflows_file" 'name: child_workflow_depth_gate' 'workflows.yaml missing G064 child workflow depth gate'
check_has_match "$ownership_file" '^  state\.json:' 'agent ownership manifest missing state.json ownership block'
check_has_match "$ownership_file" '^  scenario-manifest\.json:' 'agent ownership manifest missing scenario-manifest ownership block'
check_has_match "$capabilities_file" '^  bubbles\.validate:' 'agent capabilities manifest missing bubbles.validate entry'
check_has_match "$capabilities_file" 'certificationWriter: bubbles\.validate' 'agent capabilities manifest must declare bubbles.validate as certification writer'

check_no_match "$agents_dir/bubbles.design.agent.md" '^- `spec\.md` — Feature specification|create or complete it using the spec template' 'bubbles.design must not own or create spec.md'
check_has_match "$agents_dir/bubbles.analyst.agent.md" '\*\*Artifact Ownership:\*\*' 'bubbles.analyst must declare an Artifact Ownership block'
check_has_match "$agents_dir/bubbles.analyst.agent.md" 'MUST NOT edit `design\.md`, `scopes\.md`, `report\.md`, `uservalidation\.md`, or `state\.json\.certification\.\*`' 'bubbles.analyst must explicitly forbid foreign-artifact edits'
check_has_match "$agents_dir/bubbles.analyst.agent.md" 'review-shaped requests default to diagnostic output|mode: review|output: diagnostic' 'bubbles.analyst must distinguish review-only runs from spec-promotion runs'
check_no_match "$agents_dir/bubbles.analyst.agent.md" 'create design\.md\b|update design\.md\b|edit design\.md\b|modify.*design\.md\b' 'bubbles.analyst must not claim to create or edit design.md directly'
check_no_match "$agents_dir/bubbles.analyst.agent.md" 'create scopes\.md\b|update scopes\.md\b|edit scopes\.md\b|modify.*scopes\.md\b' 'bubbles.analyst must not claim to create or edit scopes.md directly'
check_has_match "$agents_dir/bubbles.system-review.agent.md" 'promoteFindings: true\|false' 'bubbles.system-review must expose an explicit promotion opt-in control'
check_has_match "$agents_dir/bubbles.system-review.agent.md" 'update-specs.*promoteFindings: true|explicitly requested spec creation/updates|Review language alone is not promotion permission' 'bubbles.system-review must require explicit promotion intent before routing into planning/design work'
check_no_match "$agents_dir/bubbles.system-review.agent.md" 'Use `completed_owned`|created or updated promoted spec artifacts within its owned execution surface' 'bubbles.system-review must remain diagnostic and must not claim promoted-spec success as owned work'
check_no_match "$agents_dir/bubbles.validate.agent.md" '^#### 7\.2 What to Update \(Per Issue Category\)|Artifact to Update \| What to Add' 'bubbles.validate must route artifact changes instead of editing spec/design/scopes directly'
check_no_match "$agents_dir/bubbles.ux.agent.md" 'recommend running `/bubbles\.analyst` first, but proceed' 'bubbles.ux must not proceed without analyst-owned business inputs'
check_no_match "$agents_dir/bubbles.code-review.agent.md" 'directly or via `runSubagent`' 'bubbles.code-review must dispatch specialists, not emulate them directly'
check_no_match "$agents_dir/bubbles.system-review.agent.md" 'directly or via `runSubagent`' 'bubbles.system-review must dispatch specialists, not emulate them directly'
check_no_match "$agents_dir/bubbles.implement.agent.md" 'Create `uservalidation\.md` if missing' 'bubbles.implement must not create uservalidation.md'
check_no_match "$agents_dir/bubbles.security.agent.md" 'Update scope artifacts with new DoD items|Add new Gherkin scenarios for security behaviors' 'bubbles.security must route planning changes to bubbles.plan'
check_no_match "$agents_dir/bubbles.stabilize.agent.md" 'Update scope artifacts:' 'bubbles.stabilize must route planning changes to bubbles.plan'
check_no_match "$agents_dir/bubbles.gaps.agent.md" 'Findings artifact update \(MANDATORY — Gate G031\).*update scope artifacts|Gherkin → Test Plan Sync:|Gherkin → DoD Sync:' 'bubbles.gaps must route planning changes to bubbles.plan'
check_no_match "$agents_dir/bubbles.harden.agent.md" 'Findings artifact update \(MANDATORY — Gate G031\).*update scope artifacts|Gherkin → Test Plan Sync|Gherkin → DoD Sync' 'bubbles.harden must route planning changes to bubbles.plan'
check_no_match "$agents_dir/bubbles.clarify.agent.md" 'Small fixes \(≤30 lines\):.*Fix inline within this agent' 'bubbles.clarify must not perform inline remediation'
check_no_match "$agents_dir/bubbles.regression.agent.md" 'Small fixes \(≤30 lines\):.*Fix inline within this agent|All fixes:.*directly fix' 'bubbles.regression must route follow-up work instead of fixing inline'
check_no_match "$agents_dir/bubbles.validate.agent.md" 'Do NOT emit `✅ ALL VALIDATIONS PASSED` while any `ROUTE-REQUIRED` block is present' 'bubbles.validate should rely on RESULT-ENVELOPE as the primary workflow contract'

unexpected_child_callers="$({ awk '
  /^  bubbles\./ { agent=$1; sub(":", "", agent) }
  /canInvokeChildWorkflows:[[:space:]]*true/ { print agent }
' "$capabilities_file" | grep -vE '^bubbles\.(workflow|iterate|bug)$'; } || true)"

if [[ -n "$unexpected_child_callers" ]]; then
  echo "ERROR: only orchestrators may enable child workflows; found unexpected callers:"
  echo "$unexpected_child_callers"
  errors=1
fi

for result_agent in \
  bubbles.workflow \
  bubbles.validate \
  bubbles.audit \
  bubbles.design \
  bubbles.plan \
  bubbles.gaps \
  bubbles.clarify \
  bubbles.stabilize \
  bubbles.chaos \
  bubbles.harden \
  bubbles.security \
  bubbles.regression \
  bubbles.code-review \
  bubbles.implement \
  bubbles.test \
  bubbles.docs \
  bubbles.simplify \
  bubbles.system-review
do
  check_has_match "$agents_dir/${result_agent}.agent.md" 'RESULT-ENVELOPE' "$result_agent must declare RESULT-ENVELOPE completion output"
done

# G042 enforcement: diagnostic agents must NOT contain language that permits foreign-artifact edits
for diagnostic_agent in \
  bubbles.validate \
  bubbles.audit \
  bubbles.harden \
  bubbles.gaps \
  bubbles.stabilize \
  bubbles.security \
  bubbles.code-review \
  bubbles.system-review \
  bubbles.regression \
  bubbles.clarify
do
  check_no_match "$agents_dir/${diagnostic_agent}.agent.md" \
    'update scopes\.md.*directly\b|edit scopes\.md\b|write.*scopes\.md\b|modify.*scopes\.md\b' \
    "$diagnostic_agent MUST NOT claim to directly edit scopes.md (foreign-owned by bubbles.plan)"
  check_no_match "$agents_dir/${diagnostic_agent}.agent.md" \
    'update spec\.md.*directly\b|edit spec\.md\b|write.*spec\.md\b|modify.*spec\.md\b' \
    "$diagnostic_agent MUST NOT claim to directly edit spec.md (foreign-owned by bubbles.analyst)"
  check_no_match "$agents_dir/${diagnostic_agent}.agent.md" \
    'update design\.md.*directly\b|edit design\.md\b|write.*design\.md\b|modify.*design\.md\b' \
    "$diagnostic_agent MUST NOT claim to directly edit design.md (foreign-owned by bubbles.design)"
  check_no_match "$agents_dir/${diagnostic_agent}.agent.md" \
    'create uservalidation\.md\b|write.*uservalidation\.md\b' \
    "$diagnostic_agent MUST NOT claim to create uservalidation.md (foreign-owned by bubbles.plan)"
done

# Execution agents must not claim ownership of planning artifacts
check_no_match "$agents_dir/bubbles.implement.agent.md" \
  'Create `uservalidation\.md`|create uservalidation\.md' \
  "bubbles.implement must not create uservalidation.md (foreign-owned by bubbles.plan)"
check_no_match "$agents_dir/bubbles.test.agent.md" \
  'update spec\.md\b|edit spec\.md\b|modify spec\.md\b|update design\.md\b|edit design\.md\b' \
  "bubbles.test must not claim to edit spec.md or design.md (foreign-owned)"

# Verify diagnostic agents reference artifact-ownership.md or declare foreign-artifact routing
for diagnostic_agent in \
  bubbles.validate \
  bubbles.harden \
  bubbles.gaps \
  bubbles.stabilize \
  bubbles.security \
  bubbles.regression \
  bubbles.clarify
do
  check_has_match "$agents_dir/${diagnostic_agent}.agent.md" \
    'foreign-owned|artifact-ownership|route.*to.*owner|MUST NOT.*edit.*foreign|route_required' \
    "$diagnostic_agent must reference foreign-artifact routing or artifact-ownership rules"
done

if [[ "$errors" -ne 0 ]]; then
  exit 1
fi

echo 'Agent ownership lint passed.'