#!/usr/bin/env bash
set -u
set -o pipefail

# Hermetic RED-first contract for the IMP-103 S3+S4 repository-binding
# conformance guard. A missing production guard remains RED rather than being
# treated as proof that prohibited fixtures were rejected.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
GUARD="$SCRIPT_DIR/repository-binding-conformance-guard.sh"

TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-repository-conformance.XXXXXX")" || {
  echo "repository-binding-conformance-guard-selftest: cannot create fixture root" >&2
  exit 2
}
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

assertions_passed=0
assertions_failed=0
cases_run=0
cases_passed=0
cases_red=0
case_failure_baseline=0
LAST_OUTPUT=""
LAST_RC=0
LAST_GUARD_AVAILABLE=0

pass_assertion() {
  local case_id="$1"
  local description="$2"
  assertions_passed=$((assertions_passed + 1))
  printf '  PASS [%s] %s\n' "$case_id" "$description"
}

fail_assertion() {
  local case_id="$1"
  local description="$2"
  local detail="$3"
  assertions_failed=$((assertions_failed + 1))
  printf '  FAIL [%s] contract=%s detail=%s\n' "$case_id" "$description" "$detail"
}

begin_case() {
  local case_id="$1"
  local description="$2"
  cases_run=$((cases_run + 1))
  case_failure_baseline="$assertions_failed"
  printf '\nCASE START %s\n' "$case_id"
  printf 'CONTRACT %s\n' "$description"
}

end_case() {
  local case_id="$1"
  if [[ "$assertions_failed" -eq "$case_failure_baseline" ]]; then
    cases_passed=$((cases_passed + 1))
    printf 'CASE PASS %s\n' "$case_id"
  else
    cases_red=$((cases_red + 1))
    printf 'CASE RED %s newFailures=%s\n' \
      "$case_id" "$((assertions_failed - case_failure_baseline))"
  fi
}

write_binding_contract() {
  local target="$1"
  local heading="$2"
  {
    printf '%s\n\n' "$heading"
    cat <<'EOF'

Call `repository-binding.sh preflight` and require `PREFLIGHT_COMMITTED` before
repository-local work. After `PREFLIGHT_COMMITTED`, call
`repository-binding.sh discover-specs` with the current actionable decision.
When dispatched for an owned phase, validate the inherited actionable packet
with `repository-binding.sh validate-packet` before repository-local work.

- repositoryRoot
- repositoryAlias
- repositoryResolution.sessionId
- repositoryResolution.decisionId
- repositoryResolution.controlRevision
- repositoryResolution.controlPathDigest
- repositoryResolution.authority
- repositoryResolution.transition
- repositoryResolution.scopeKind
- repositoryResolution.scopeId
- repositoryResolution.targetKind
- repositoryResolution.pathVisibility
- repositoryResolution.actionable
EOF
  } >"$target"
}

write_binding_fields() {
  local target="$1"
  write_binding_contract "$target" "## Repository Binding Producer And Consumer Contract"
}

append_valid_compaction_contract() {
  local target="$1"
  cat >>"$target" <<'EOF'

## Context Compaction Discipline (Orchestrator Agents)

Repository-sensitive compaction calls `context-compactor.sh` with
`--binding-packet-file`, preserves the exact nested `repositoryResolution`, and
calls `repository-binding.sh validate-packet` before resume.
EOF
}

append_valid_orchestrator_compaction_contract() {
  local target="$1"
  cat >>"$target" <<'EOF'

Repository-sensitive compaction runs `context-compactor.sh` with
`--session-id`, `--session-control-file`, and `--binding-packet-file`; resume
calls `repository-binding.sh validate-packet` before repository-local work.
EOF
}

stage_clean_fixture() {
  local fixture_id="$1"
  local root="$TMP_ROOT/$fixture_id"

  mkdir -p "$root/agents/bubbles_shared" "$root/bubbles/scripts" "$root/bubbles/workflows" \
    "$root/skills/bubbles-result-envelope" || return 1
  cat >"$root/agents/bubbles.workflow.agent.md" <<'EOF'
## Execution Model

### Repository Binding Preflight

Execute `repository-binding.sh preflight`, then record `PREFLIGHT_COMMITTED`.
After `PREFLIGHT_COMMITTED`, call `repository-binding.sh discover-specs`.

### Phase -1: Intent Resolution (MANDATORY — runs before Phase 0)

Classify requests as `STRUCTURED`, `TARGETLESS_MODE`, `CONTINUATION`, or `VAGUE`.
If `mode:` is present without a concrete target, classify the request as `TARGETLESS_MODE`.

### Phase 0: Resolve Inputs

Read state.json only after repository binding commits.

#### FRAMEWORK-ENVELOPE Consumer Contract

Validate with `repository-binding.sh validate-packet` and require the returned
binding to remain unchanged.

- repositoryRoot
- repositoryAlias
- repositoryResolution.sessionId
- repositoryResolution.decisionId
- repositoryResolution.controlRevision
- repositoryResolution.controlPathDigest
- repositoryResolution.authority
- repositoryResolution.transition
- repositoryResolution.scopeKind
- repositoryResolution.scopeId
- repositoryResolution.targetKind
- repositoryResolution.pathVisibility
- repositoryResolution.actionable
EOF
  cat >"$root/agents/bubbles.iterate.agent.md" <<'EOF'
## Repository Binding Entry Contract (NON-NEGOTIABLE)

Execute `repository-binding.sh preflight`, then record `PREFLIGHT_COMMITTED`.

## Execution Flow

### Repository Binding Preflight

Execute `repository-binding.sh preflight`, then record `PREFLIGHT_COMMITTED`.
After `PREFLIGHT_COMMITTED`, call `repository-binding.sh discover-specs`.

### Phase 0: Context Resolution

Read state.json only after repository binding commits.
EOF
  cat >"$root/agents/bubbles.super.agent.md" <<'EOF'
### Subagent Response Contract (when invoked via `runSubagent`)

Resolution is two-stage. Resolve repository intent from bounded candidate descriptors only; do not receive a cross-repository specs listing.
Execute repository binding with `--resolved-natural-language-root` and require `PREFLIGHT_COMMITTED`.
After `PREFLIGHT_COMMITTED`, resolve work only under `resolvedRepositoryRoot/specs`.

#### FRAMEWORK-ENVELOPE Repository Binding Contract

Validate with `repository-binding.sh validate-packet` before operation execution.

- repositoryRoot
- repositoryAlias
- repositoryResolution.sessionId
- repositoryResolution.decisionId
- repositoryResolution.controlRevision
- repositoryResolution.controlPathDigest
- repositoryResolution.authority
- repositoryResolution.transition
- repositoryResolution.scopeKind
- repositoryResolution.scopeId
- repositoryResolution.targetKind
- repositoryResolution.pathVisibility
- repositoryResolution.actionable
EOF
  write_binding_contract "$root/agents/bubbles.recap.agent.md" "## CONTINUATION-ENVELOPE"
  write_binding_contract "$root/agents/bubbles.status.agent.md" "## CONTINUATION-ENVELOPE"
  write_binding_contract "$root/agents/bubbles.handoff.agent.md" '## Step 1: The "Handoff" Prompt'
  write_binding_contract "$root/agents/bubbles_shared/agent-common.md" \
    "## Workflow-Only Continuation Convention (NON-NEGOTIABLE)"
  cat >>"$root/agents/bubbles_shared/agent-common.md" <<'EOF'

## Repository Binding Entry Contract (NON-NEGOTIABLE)

Direct top-level invocation executes `repository-binding.sh preflight` and
requires `PREFLIGHT_COMMITTED`. Dispatched phase-owner invocation executes
`repository-binding.sh validate-packet` against authoritative host-private session control.
Stale and root-substituted packets refuse with zero repository-local side effects.
EOF
  write_binding_contract "$root/skills/bubbles-result-envelope/SKILL.md" \
    "## Repository Binding (repository-sensitive invocations)"
  cat >"$root/bubbles/agent-capabilities.yaml" <<'EOF'
agents:
  bubbles.implement:
    class: execution-owner
    ownsPhases: [ implement ]
  bubbles.bug:
    class: orchestrator
    ownsPhases: [ bug-discovery ]

workflowModeGrants:
  defaultAllowed: false
  agents:
    bubbles.workflow:
      modes: [ "*" ]
    bubbles.goal:
      modes: [ "*" ]
    bubbles.sprint:
      modes: [ "*" ]
    bubbles.iterate:
      modes: [ iterate ]
    bubbles.bug:
      modes: [ bugfix-fastlane ]
    bubbles.releases:
      modes: [ release-planning-to-doc ]
    bubbles.train:
      modes: [ release-train-cut ]
    bubbles.upkeep:
      modes: [ upkeep-backup-verify ]
    bubbles.propagate:
      modes: [ propagate-forward ]
    bubbles.stabilize:
      modes: [ stabilize-to-doc ]
    bubbles.retro:
      modes: [ retro-to-review ]
    bubbles.journey:
      modes: [ journey-refinement ]
EOF
  cat >"$root/agents/bubbles.implement.agent.md" <<'EOF'
## Repository Binding Entry Contract (NON-NEGOTIABLE)

Follow [agent-common.md](bubbles_shared/agent-common.md#repository-binding-entry-contract-non-negotiable).
Direct invocation runs `repository-binding.sh preflight` and requires
`PREFLIGHT_COMMITTED`. Dispatched invocation runs
`repository-binding.sh validate-packet` before repository-local work.

## Agent Identity

Implementation owner.

## Execution Flow

Repository-local work starts here.
EOF
  for runner in \
    bubbles.bug bubbles.releases bubbles.train bubbles.upkeep bubbles.propagate \
    bubbles.stabilize bubbles.retro bubbles.journey; do
    write_binding_contract "$root/agents/$runner.agent.md" "## Repository Binding (NON-NEGOTIABLE)"
    printf '\n## User Input\n\nRepository-local input begins after preflight.\n' >>"$root/agents/$runner.agent.md"
  done
  cat >"$root/agents/bubbles.goal.agent.md" <<'EOF'
## Repository Binding Entry Contract (NON-NEGOTIABLE)

Execute `repository-binding.sh preflight` and require `PREFLIGHT_COMMITTED`.

## PHASE ROUTER (EXECUTE TOP-TO-BOTTOM)

phase_1_understand:
  do: understand the goal

## Goal Scenario Compilation (Cross-Repo / Multi-Phase)

### Repository Binding For Goal Nodes

Execute `repository-binding.sh preflight` and require `PREFLIGHT_COMMITTED`.
Each node uses `scopeKind: goal-node` and sets `scopeId` to the node id, then calls
`repository-binding.sh validate-packet --scenario-file <compiled-scenario.json>
--node-id <node-id>` before dispatch.
After every node, verify the top-level root and revision remain byte-identical.
EOF
  cat >"$root/agents/bubbles.sprint.agent.md" <<'EOF'
## Repository Binding Entry Contract (NON-NEGOTIABLE)

Execute `repository-binding.sh preflight` and require `PREFLIGHT_COMMITTED`.

## PHASE ROUTER (EXECUTE TOP-TO-BOTTOM)

phase_1_parse_and_estimate:
  do: build the queue

## Sprint Scenario Execution (Cross-Repo / Multi-Phase Missions)

### Repository Binding For Sprint Nodes

Execute `repository-binding.sh preflight` and require `PREFLIGHT_COMMITTED`.
Each node uses `scopeKind: goal-node` and sets `scopeId` to the node id, then calls
`repository-binding.sh validate-packet --scenario-file <compiled-scenario.json>
--node-id <node-id>` before dispatch.
After every node, verify the top-level root and revision remain byte-identical.
EOF
  append_valid_orchestrator_compaction_contract "$root/agents/bubbles.workflow.agent.md"
  append_valid_orchestrator_compaction_contract "$root/agents/bubbles.iterate.agent.md"
  append_valid_orchestrator_compaction_contract "$root/agents/bubbles.goal.agent.md"
  append_valid_orchestrator_compaction_contract "$root/agents/bubbles.sprint.agent.md"
  cat >"$root/agents/bubbles_shared/scenario-compile.md" <<'EOF'
## Scenario DAG Schema

repos:
  - id: product
    repositoryRoot: <canonical-absolute-git-root>
    repositoryAlias: <safe-local-display-alias>

Every repository declaration carries its canonical repositoryRoot and safe repositoryAlias.
EOF
  cat >"$root/agents/bubbles_shared/workflow-delegation-core.md" <<'EOF'
## Workflow Delegation Core

### Input Classification Contract

Repository preflight commits `PREFLIGHT_COMMITTED` before repository-local work.
- Literal `mode:` plus a concrete spec/bug/ops target is `STRUCTURED`.
- Literal `mode:` plus `repositoryRoot` without a concrete target is `TARGETLESS_MODE`.
- Literal `mode:` without a concrete target is `TARGETLESS_MODE`, never `STRUCTURED`.
- Without `mode:`, `CONTINUATION` and `VAGUE` retain their semantics after preflight.

Chat CWD, prompt source, active editor, tool CWD, and host repository metadata are diagnostic-only and never authority.

### Envelope Consumption Rules

FRAMEWORK-ENVELOPE binding is validated with `repository-binding.sh validate-packet` before reporting.
EOF
  cat >"$root/agents/bubbles_shared/workflow-execution-loops.md" <<'EOF'
## Workflow Execution Loops

### Stochastic And Iterate Discovery

Require `PREFLIGHT_COMMITTED`, then call `repository-binding.sh discover-specs`.
The executable discovery scope is `resolvedRepositoryRoot/specs`.
EOF
  write_binding_fields "$root/agents/bubbles_shared/workflow-input-bootstrap.md"
  write_binding_fields "$root/agents/bubbles_shared/workflow-phase-engine.md"
  cat >"$root/agents/bubbles_shared/operating-baseline.md" <<'EOF'
## Repository Authority Baseline

Prompt source, chat CWD, active editor, tool CWD, host repository metadata, workspace declaration order, the first workspace root, and recent work are diagnostic-only. They can never establish, switch, repair, or override repository authority.

## Context Compaction Discipline (Orchestrator Agents)

Repository-sensitive compaction calls `context-compactor.sh` with
`--binding-packet-file`, preserves the exact nested `repositoryResolution`, and
calls `repository-binding.sh validate-packet` before resume.
EOF
  cat >"$root/bubbles/scripts/context-compactor.sh" <<'EOF'
#!/usr/bin/env bash
REPOSITORY_BINDING="repository-binding.sh"
"$REPOSITORY_BINDING" validate-packet --packet-file "$BINDING_PACKET_FILE"
printf '"repositoryResolution":{'
EOF
  cat >"$root/bubbles/workflows/modes.yaml" <<'EOF'
modes:
  stochastic-quality-sweep:
    constraints:
      autoDiscoverAllSpecs: true
      repositoryPreflightRequired: true
      discoveryScope: resolvedRepositoryRoot/specs
  iterate:
    constraints:
      autoDiscoverAllSpecs: true
      repositoryPreflightRequired: true
      discoveryScope: resolvedRepositoryRoot/specs
EOF
  printf '%s\n' "$root"
}

run_guard() {
  local fixture_root="$1"
  printf 'COMMAND bash %s --root %s\n' "$GUARD" "$fixture_root"
  if [[ ! -f "$GUARD" ]]; then
    LAST_GUARD_AVAILABLE=0
    LAST_RC=127
    LAST_OUTPUT="repository-binding-conformance RED productionInterfaceUnavailable=$GUARD"
  else
    LAST_GUARD_AVAILABLE=1
    LAST_OUTPUT="$(bash "$GUARD" --root "$fixture_root" 2>&1)"
    LAST_RC=$?
  fi
  printf '%s\n' "$LAST_OUTPUT"
  printf 'EXIT %s\n' "$LAST_RC"
}

assert_guard_exit() {
  local case_id="$1"
  local description="$2"
  local expected="$3"
  if [[ "$LAST_GUARD_AVAILABLE" -ne 1 ]]; then
    fail_assertion "$case_id" "$description" "productionInterfaceUnavailable=true expectedExit=$expected"
  elif [[ "$LAST_RC" -eq "$expected" ]]; then
    pass_assertion "$case_id" "$description"
  else
    fail_assertion "$case_id" "$description" "expectedExit=$expected actualExit=$LAST_RC"
  fi
}

assert_guard_reports() {
  local case_id="$1"
  local description="$2"
  local expected="$3"
  if [[ "$LAST_GUARD_AVAILABLE" -ne 1 ]]; then
    fail_assertion "$case_id" "$description" "productionInterfaceUnavailable=true expectedOutput=$expected"
  else
    case "$LAST_OUTPUT" in
      *"$expected"*) pass_assertion "$case_id" "$description" ;;
      *) fail_assertion "$case_id" "$description" "missingOutput=$expected" ;;
    esac
  fi
}

echo "=== IMP-103 S3+S4 repository-binding-conformance-guard selftest ==="
echo "PRODUCTION guard=$GUARD"

case_id="RB-CONFORMANCE-CLEAN"
begin_case "$case_id" "A source fixture with targetless classification, preflight ordering, scoped discovery, complete fields, and diagnostic-only ambient signals passes."
fixture="$(stage_clean_fixture clean)" || exit 2
run_guard "$fixture"
assert_guard_exit "$case_id" "clean fixture passes" 0
end_case "$case_id"

case_id="RB-CONFORMANCE-MODE-ONLY-STRUCTURED"
begin_case "$case_id" "Active mode-only text described as STRUCTURED is rejected."
fixture="$(stage_clean_fixture mode-only-structured)" || exit 2
cat >"$fixture/agents/bubbles_shared/workflow-delegation-core.md" <<'EOF'
## Workflow Delegation Core

### Input Classification Contract

Literal `mode:` alone is `TARGETLESS_MODE` and `STRUCTURED` and may proceed directly.
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "mode-only STRUCTURED fixture fails" 1
assert_guard_reports "$case_id" "mode-only failure is identified" "RB-CONFORMANCE-MODE-ONLY-STRUCTURED"
end_case "$case_id"

case_id="RB-CONFORMANCE-RAW-SPECS-DISCOVERY"
begin_case "$case_id" "Raw unqualified executable specs auto-discovery is rejected."
fixture="$(stage_clean_fixture raw-specs)" || exit 2
cat >"$fixture/agents/bubbles_shared/workflow-execution-loops.md" <<'EOF'
## Workflow Execution Loops

### Stochastic And Iterate Discovery

Discover every spec folder under `specs/` as the executable pool.
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "raw specs discovery fixture fails" 1
assert_guard_reports "$case_id" "raw specs failure is identified" "RB-CONFORMANCE-RAW-SPECS-DISCOVERY"
end_case "$case_id"

case_id="RB-CONFORMANCE-PREFLIGHT-ANCHOR-MISSING"
begin_case "$case_id" "Discovery without a preceding preflight commit anchor is rejected."
fixture="$(stage_clean_fixture missing-preflight)" || exit 2
cat >"$fixture/agents/bubbles_shared/workflow-execution-loops.md" <<'EOF'
## Workflow Execution Loops

### Stochastic And Iterate Discovery

Call `repository-binding.sh discover-specs` with an actionable decision.
The executable discovery scope is `resolvedRepositoryRoot/specs`.
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "missing preflight anchor fixture fails" 1
assert_guard_reports "$case_id" "missing preflight failure is identified" "RB-CONFORMANCE-PREFLIGHT-ANCHOR-MISSING"
end_case "$case_id"

case_id="RB-CONFORMANCE-FRONT-DOOR-STATE-BEFORE-PREFLIGHT"
begin_case "$case_id" "Workflow front-door state access before repository preflight is rejected."
fixture="$(stage_clean_fixture front-door-state-before-preflight)" || exit 2
cat >"$fixture/agents/bubbles.workflow.agent.md" <<'EOF'
## Execution Model

Read state.json before repository selection.

### Repository Binding Preflight

Execute `repository-binding.sh preflight`, then record `PREFLIGHT_COMMITTED`.
After `PREFLIGHT_COMMITTED`, call `repository-binding.sh discover-specs`.
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "front-door state-before-preflight fixture fails" 1
assert_guard_reports "$case_id" "front-door ordering failure is identified" "RB-CONFORMANCE-FRONT-DOOR-ORDER"
end_case "$case_id"

case_id="RB-CONFORMANCE-SUPER-PREBINDING-SPECS"
begin_case "$case_id" "A pre-binding specs listing supplied to super is rejected."
fixture="$(stage_clean_fixture super-prebinding-specs)" || exit 2
cat >"$fixture/agents/bubbles.super.agent.md" <<'EOF'
### Subagent Response Contract (when invoked via `runSubagent`)

Resolution is two-stage. Input includes Available specs: {specs/ listing} before repository selection.
Execute repository binding and require `PREFLIGHT_COMMITTED`.
After `PREFLIGHT_COMMITTED`, resolve work only under `resolvedRepositoryRoot/specs`.
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "super pre-binding specs fixture fails" 1
assert_guard_reports "$case_id" "super pre-binding specs failure is identified" "RB-CONFORMANCE-SUPER-PREBINDING-SPECS"
end_case "$case_id"

case_id="RB-CONFORMANCE-CONTINUATION-FIELDS-DROPPED"
begin_case "$case_id" "A continuation contract that drops one exact binding field is rejected."
fixture="$(stage_clean_fixture continuation-fields-dropped)" || exit 2
cat >"$fixture/agents/bubbles.recap.agent.md" <<'EOF'
## CONTINUATION-ENVELOPE

- repositoryRoot
- repositoryAlias
- repositoryResolution.sessionId
- repositoryResolution.decisionId
- repositoryResolution.authority
- repositoryResolution.transition
- repositoryResolution.scopeKind
- repositoryResolution.scopeId
- repositoryResolution.targetKind
- repositoryResolution.pathVisibility
- repositoryResolution.actionable
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "continuation field-drop fixture fails" 1
assert_guard_reports "$case_id" "continuation field-drop failure is identified" "RB-CONFORMANCE-BINDING-FIELDS-DROPPED"
end_case "$case_id"

case_id="RB-CONFORMANCE-CONTROL-PATH-DIGEST-DROPPED"
begin_case "$case_id" "A repository-binding contract that drops only controlPathDigest is rejected."
fixture="$(stage_clean_fixture control-path-digest-dropped)" || exit 2
cat >"$fixture/agents/bubbles.recap.agent.md" <<'EOF'
## CONTINUATION-ENVELOPE

- repositoryRoot
- repositoryAlias
- repositoryResolution.sessionId
- repositoryResolution.decisionId
- repositoryResolution.controlRevision
- repositoryResolution.authority
- repositoryResolution.transition
- repositoryResolution.scopeKind
- repositoryResolution.scopeId
- repositoryResolution.targetKind
- repositoryResolution.pathVisibility
- repositoryResolution.actionable
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "control-path digest field-drop fixture fails" 1
assert_guard_reports "$case_id" "control-path digest failure is identified" "missing-field=repositoryResolution.controlPathDigest"
end_case "$case_id"

case_id="RB-CONFORMANCE-STATE-SNAPSHOT-UNBOUND"
begin_case "$case_id" "A repository-local state-snapshot invocation without the complete binding triplet is rejected."
fixture="$(stage_clean_fixture state-snapshot-unbound)" || exit 2
cat >>"$fixture/agents/bubbles.goal.agent.md" <<'EOF'

Every iteration runs `bash bubbles/scripts/state-snapshot.sh --convergence-iteration <N> --spec-dir <specDir>`.
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "unbound state-snapshot fixture fails" 1
assert_guard_reports "$case_id" "unbound state-snapshot failure is identified" "RB-CONFORMANCE-STATE-SNAPSHOT-UNBOUND"
end_case "$case_id"

case_id="RB-CONFORMANCE-COMPACTION-RESUME-UNPORTED"
begin_case "$case_id" "A compaction contract that omits bound invocation and exact packet validation on resume is rejected."
fixture="$(stage_clean_fixture compaction-resume-unported)" || exit 2
cat >"$fixture/agents/bubbles_shared/operating-baseline.md" <<'EOF'
## Repository Authority Baseline

Ambient signals are diagnostic-only and never repository authority.

## Context Compaction Discipline (Orchestrator Agents)

Run `context-compactor.sh <raw-result-file>` and continue from its summary.
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "unbound compaction/resume fixture fails" 1
assert_guard_reports "$case_id" "compaction/resume omission is identified" "RB-CONFORMANCE-COMPACTION-RESUME-UNPORTED"
end_case "$case_id"

case_id="RB-CONFORMANCE-ORCHESTRATOR-COMPACTION-UNBOUND"
begin_case "$case_id" "A repository-sensitive orchestrator that retains the legacy unbound compactor command is rejected."
fixture="$(stage_clean_fixture orchestrator-compaction-unbound)" || exit 2
cat >>"$fixture/agents/bubbles.iterate.agent.md" <<'EOF'

Use `bash bubbles/scripts/context-compactor.sh <raw-envelope-file>` and continue.
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "unbound orchestrator compaction fixture fails" 1
assert_guard_reports "$case_id" "unbound orchestrator compaction is identified" "RB-CONFORMANCE-ORCHESTRATOR-COMPACTION-UNBOUND"
end_case "$case_id"

case_id="RB-CONFORMANCE-FRAMEWORK-FIELDS-DROPPED"
begin_case "$case_id" "A FRAMEWORK-ENVELOPE producer that drops one exact binding field is rejected."
fixture="$(stage_clean_fixture framework-fields-dropped)" || exit 2
cat >"$fixture/agents/bubbles.super.agent.md" <<'EOF'
### Subagent Response Contract (when invoked via `runSubagent`)

Resolution is two-stage. Resolve repository intent from bounded candidate descriptors only; do not receive a cross-repository specs listing.
Execute repository binding and require `PREFLIGHT_COMMITTED`.
After `PREFLIGHT_COMMITTED`, resolve work only under `resolvedRepositoryRoot/specs`.

#### FRAMEWORK-ENVELOPE Repository Binding Contract

Validate with `repository-binding.sh validate-packet` before operation execution.

- repositoryRoot
- repositoryAlias
- repositoryResolution.sessionId
- repositoryResolution.decisionId
- repositoryResolution.authority
- repositoryResolution.transition
- repositoryResolution.scopeKind
- repositoryResolution.scopeId
- repositoryResolution.targetKind
- repositoryResolution.pathVisibility
- repositoryResolution.actionable
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "FRAMEWORK field-drop fixture fails" 1
assert_guard_reports "$case_id" "FRAMEWORK field-drop failure is identified" "RB-CONFORMANCE-BINDING-FIELDS-DROPPED"
end_case "$case_id"

case_id="RB-CONFORMANCE-RESULT-FIELDS-DROPPED"
begin_case "$case_id" "The result-envelope skill dropping one exact binding field is rejected."
fixture="$(stage_clean_fixture result-fields-dropped)" || exit 2
cat >"$fixture/skills/bubbles-result-envelope/SKILL.md" <<'EOF'
## Repository Binding (repository-sensitive invocations)

- repositoryRoot
- repositoryAlias
- repositoryResolution.sessionId
- repositoryResolution.decisionId
- repositoryResolution.controlRevision
- repositoryResolution.controlPathDigest
- repositoryResolution.authority
- repositoryResolution.transition
- repositoryResolution.scopeKind
- repositoryResolution.scopeId
- repositoryResolution.targetKind
- repositoryResolution.pathVisibility
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "result field-drop fixture fails" 1
assert_guard_reports "$case_id" "result field-drop failure is identified" "RB-CONFORMANCE-BINDING-FIELDS-DROPPED"
end_case "$case_id"

case_id="RB-CONFORMANCE-REGISTRY-RUNNER-UNPORTED"
begin_case "$case_id" "A direct runner added to workflowModeGrants without a binding contract is rejected."
fixture="$(stage_clean_fixture registry-runner-unported)" || exit 2
cat >>"$fixture/bubbles/agent-capabilities.yaml" <<'EOF'
    bubbles.unported:
      modes: [ full-delivery ]
EOF
cat >"$fixture/agents/bubbles.unported.agent.md" <<'EOF'
## Agent Identity

This registry-derived runner has no repository binding contract.
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "registry-derived unported runner fixture fails" 1
assert_guard_reports "$case_id" "unported runner failure is identified" "RB-CONFORMANCE-DIRECT-RUNNER-UNPORTED"
end_case "$case_id"

case_id="RB-CONFORMANCE-REGISTRY-RUNNER-LATE-PREFLIGHT"
begin_case "$case_id" "A workflowModeGrants-derived runner whose repository work precedes preflight is rejected even when a later subsection contains every anchor."
fixture="$(stage_clean_fixture registry-runner-late-preflight)" || exit 2
cat >"$fixture/agents/bubbles.goal.agent.md" <<'EOF'
## PHASE ROUTER (EXECUTE TOP-TO-BOTTOM)

phase_1_understand:
  do: read files and classify the goal

## Goal Scenario Compilation (Cross-Repo / Multi-Phase)

### Repository Binding For Goal Nodes

Execute `repository-binding.sh preflight` and require `PREFLIGHT_COMMITTED`.
Each node uses `scopeKind: goal-node`, sets `scopeId`, and verifies the top-level
root and revision remain byte-identical.
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "late direct-runner preflight fixture fails" 1
assert_guard_reports "$case_id" "late direct-runner failure is identified" "RB-CONFORMANCE-DIRECT-RUNNER-UNPORTED"
end_case "$case_id"

case_id="RB-CONFORMANCE-PHASE-OWNER-PACKET-VALIDATION-MISSING"
begin_case "$case_id" "A capabilities-derived phase owner that can read local state without validating an inherited packet is rejected."
fixture="$(stage_clean_fixture phase-owner-validation-missing)" || exit 2
cat >"$fixture/agents/bubbles.implement.agent.md" <<'EOF'
## Repository Binding Entry Contract (NON-NEGOTIABLE)

Follow [agent-common.md](bubbles_shared/agent-common.md#repository-binding-entry-contract-non-negotiable).
Direct invocation runs `repository-binding.sh preflight` and requires
`PREFLIGHT_COMMITTED`.

## Agent Identity

Implementation owner.

## Execution Flow

Read repository-local state.
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "missing inherited-packet validation fixture fails" 1
assert_guard_reports "$case_id" "phase-owner packet failure is identified" "RB-CONFORMANCE-PHASE-OWNER-ENTRY-UNPORTED"
end_case "$case_id"

case_id="RB-CONFORMANCE-DUAL-ROLE-BUG-PACKET-VALIDATION-MISSING"
begin_case "$case_id" "A workflowModeGrants runner that also owns bug-discovery must retain inherited-packet validation as well as top-level preflight."
fixture="$(stage_clean_fixture dual-role-bug-validation-missing)" || exit 2
cat >"$fixture/agents/bubbles.bug.agent.md" <<'EOF'
## Repository Binding (NON-NEGOTIABLE)

Top-level bug workflow execution runs `repository-binding.sh preflight` and
requires `PREFLIGHT_COMMITTED` before repository-local work.

## User Input

Bug discovery begins here.
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "dual-role bubbles.bug without inherited validation fails" 1
assert_guard_reports "$case_id" "dual-role bubbles.bug failure is identified" "RB-CONFORMANCE-PHASE-OWNER-ENTRY-UNPORTED"
end_case "$case_id"

case_id="RB-CONFORMANCE-SHARED-ENTRY-CONTRACT-MISSING"
begin_case "$case_id" "Phase-owner links cannot pass when the reusable shared top-level-or-inherited entry contract is absent."
fixture="$(stage_clean_fixture shared-entry-contract-missing)" || exit 2
cat >"$fixture/agents/bubbles_shared/agent-common.md" <<'EOF'
## Workflow-Only Continuation Convention (NON-NEGOTIABLE)

- repositoryRoot
- repositoryAlias
- repositoryResolution.sessionId
- repositoryResolution.decisionId
- repositoryResolution.controlRevision
- repositoryResolution.controlPathDigest
- repositoryResolution.authority
- repositoryResolution.transition
- repositoryResolution.scopeKind
- repositoryResolution.scopeId
- repositoryResolution.targetKind
- repositoryResolution.pathVisibility
- repositoryResolution.actionable
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "missing shared entry contract fixture fails" 1
assert_guard_reports "$case_id" "missing shared entry contract is identified" "RB-CONFORMANCE-SHARED-ENTRY-CONTRACT-MISSING"
end_case "$case_id"

case_id="RB-CONFORMANCE-WORKFLOW-ACTIVE-MODE-ONLY-STRUCTURED"
begin_case "$case_id" "The workflow agent active classifier cannot prefer STRUCTURED or omit TARGETLESS_MODE while the shared classifier remains correct."
fixture="$(stage_clean_fixture workflow-active-mode-only-structured)" || exit 2
cat >"$fixture/agents/bubbles.workflow.agent.md" <<'EOF'
## Execution Model

### Repository Binding Preflight

Execute `repository-binding.sh preflight`, then record `PREFLIGHT_COMMITTED`.
After `PREFLIGHT_COMMITTED`, call `repository-binding.sh discover-specs`.

### Phase -1: Intent Resolution (MANDATORY — runs before Phase 0)

If classification is ambiguous, prefer STRUCTURED.

### Phase 0: Resolve Inputs

Read state.json only after repository binding commits.

#### FRAMEWORK-ENVELOPE Consumer Contract

Validate with `repository-binding.sh validate-packet` and preserve the exact binding.
- repositoryRoot
- repositoryAlias
- repositoryResolution.sessionId
- repositoryResolution.decisionId
- repositoryResolution.controlRevision
- repositoryResolution.controlPathDigest
- repositoryResolution.authority
- repositoryResolution.transition
- repositoryResolution.scopeKind
- repositoryResolution.scopeId
- repositoryResolution.targetKind
- repositoryResolution.pathVisibility
- repositoryResolution.actionable
EOF
append_valid_orchestrator_compaction_contract "$fixture/agents/bubbles.workflow.agent.md"
run_guard "$fixture"
assert_guard_exit "$case_id" "workflow active classifier fixture fails" 1
assert_guard_reports "$case_id" "workflow active classifier failure is identified" "RB-CONFORMANCE-WORKFLOW-CLASSIFIER-MISSING"
end_case "$case_id"

case_id="RB-CONFORMANCE-WORKFLOW-ACTIVE-RAW-SPECS"
begin_case "$case_id" "The workflow agent cannot retain an executable raw specs auto-discovery instruction behind a valid preflight subsection."
fixture="$(stage_clean_fixture workflow-active-raw-specs)" || exit 2
cat >>"$fixture/agents/bubbles.workflow.agent.md" <<'EOF'

Auto-discover all spec folders under `specs/` when no targets are provided.
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "workflow active raw-specs fixture fails" 1
assert_guard_reports "$case_id" "workflow active raw-specs failure is identified" "RB-CONFORMANCE-ACTIVE-RAW-SPECS"
end_case "$case_id"

case_id="RB-CONFORMANCE-SCENARIO-REPOSITORY-ROOT-DROPPED"
begin_case "$case_id" "A scenario contract that drops canonical repositoryRoot is rejected."
fixture="$(stage_clean_fixture scenario-repository-root-dropped)" || exit 2
cat >"$fixture/agents/bubbles_shared/scenario-compile.md" <<'EOF'
## Scenario DAG Schema

repos:
  - id: product
    role: product
    repositoryAlias: <safe-local-display-alias>
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "scenario repositoryRoot-drop fixture fails" 1
assert_guard_reports "$case_id" "scenario repositoryRoot-drop failure is identified" "RB-CONFORMANCE-SCENARIO-REPOSITORY-ROOT-DROPPED"
end_case "$case_id"

case_id="RB-CONFORMANCE-SCENARIO-REPOSITORY-ALIAS-DROPPED"
begin_case "$case_id" "A scenario contract that drops declaration-bound repositoryAlias is rejected."
fixture="$(stage_clean_fixture scenario-repository-alias-dropped)" || exit 2
cat >"$fixture/agents/bubbles_shared/scenario-compile.md" <<'EOF'
## Scenario DAG Schema

repos:
  - id: product
    role: product
    repositoryRoot: <canonical-absolute-git-root>
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "scenario repositoryAlias-drop fixture fails" 1
assert_guard_reports "$case_id" "scenario repositoryAlias-drop failure is identified" "RB-CONFORMANCE-SCENARIO-REPOSITORY-ALIAS-DROPPED"
end_case "$case_id"

case_id="RB-CONFORMANCE-GOAL-NODE-CONTRACT-DROPPED"
begin_case "$case_id" "A goal executor that drops declaration-bound packet validation, node scope, and top-level invariance is rejected."
fixture="$(stage_clean_fixture goal-node-contract-dropped)" || exit 2
cat >"$fixture/agents/bubbles.goal.agent.md" <<'EOF'
## Goal Scenario Compilation (Cross-Repo / Multi-Phase)

### Repository Binding For Goal Nodes

Execute `repository-binding.sh preflight` and require `PREFLIGHT_COMMITTED`.
Dispatch nodes using their symbolic repo id.
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "goal-node declaration/scope/invariance-drop fixture fails" 1
assert_guard_reports "$case_id" "goal-node contract failure is identified" "RB-CONFORMANCE-GOAL-NODE-CONTRACT-DROPPED"
end_case "$case_id"

case_id="RB-CONFORMANCE-PRODUCER-FIELDS-DROPPED"
begin_case "$case_id" "A producer contract that drops a repository-binding field is rejected."
fixture="$(stage_clean_fixture producer-fields-dropped)" || exit 2
cat >"$fixture/agents/bubbles_shared/workflow-input-bootstrap.md" <<'EOF'
## Repository Binding Producer Contract

Call `repository-binding.sh preflight` and require `PREFLIGHT_COMMITTED` before
repository-local work. After `PREFLIGHT_COMMITTED`, call
`repository-binding.sh discover-specs` with the current actionable decision.

- repositoryRoot
- repositoryAlias
- repositoryResolution.sessionId
- repositoryResolution.decisionId
- repositoryResolution.authority
- repositoryResolution.transition
- repositoryResolution.scopeKind
- repositoryResolution.scopeId
- repositoryResolution.targetKind
- repositoryResolution.pathVisibility
- repositoryResolution.actionable
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "producer field-drop fixture fails" 1
assert_guard_reports "$case_id" "producer field-drop failure is identified" "RB-CONFORMANCE-BINDING-FIELDS-DROPPED"
end_case "$case_id"

case_id="RB-CONFORMANCE-CONSUMER-FIELDS-DROPPED"
begin_case "$case_id" "A consumer contract that drops a repository-binding field is rejected."
fixture="$(stage_clean_fixture consumer-fields-dropped)" || exit 2
cat >"$fixture/agents/bubbles_shared/workflow-phase-engine.md" <<'EOF'
## Repository Binding Consumer Contract

Call `repository-binding.sh preflight` and require `PREFLIGHT_COMMITTED` before
repository-local work. After `PREFLIGHT_COMMITTED`, call
`repository-binding.sh discover-specs` with the current actionable decision.

- repositoryRoot
- repositoryAlias
- repositoryResolution.sessionId
- repositoryResolution.decisionId
- repositoryResolution.controlRevision
- repositoryResolution.controlPathDigest
- repositoryResolution.authority
- repositoryResolution.transition
- repositoryResolution.scopeKind
- repositoryResolution.scopeId
- repositoryResolution.targetKind
- repositoryResolution.actionable
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "consumer field-drop fixture fails" 1
assert_guard_reports "$case_id" "consumer field-drop failure is identified" "RB-CONFORMANCE-BINDING-FIELDS-DROPPED"
end_case "$case_id"

case_id="RB-CONFORMANCE-AMBIENT-AUTHORITY-CWD"
begin_case "$case_id" "CWD, prompt, editor, or host metadata described as repository authority is rejected."
fixture="$(stage_clean_fixture ambient-authority)" || exit 2
cat >"$fixture/agents/bubbles_shared/operating-baseline.md" <<'EOF'
## Repository Authority Baseline

Chat CWD is repository authority. Prompt source, active editor, and host repository metadata break ties when selecting a repository.
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "ambient authority fixture fails" 1
assert_guard_reports "$case_id" "ambient authority failure is identified" "RB-CONFORMANCE-AMBIENT-AUTHORITY"
assert_guard_reports "$case_id" "ambient authority diagnostic identifies CWD" "signal=chat-cwd"
end_case "$case_id"

case_id="RB-CONFORMANCE-AMBIENT-AUTHORITY-WORKSPACE-ORDER"
begin_case "$case_id" "Workspace declaration order cannot select a repository or break ties."
fixture="$(stage_clean_fixture workspace-order-authority)" || exit 2
cat >"$fixture/agents/bubbles_shared/operating-baseline.md" <<'EOF'
## Repository Authority Baseline

Workspace declaration order selects the repository and breaks ties between eligible roots.
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "workspace-order authority fixture fails" 1
assert_guard_reports "$case_id" "workspace-order failure is identified" "RB-CONFORMANCE-AMBIENT-AUTHORITY"
assert_guard_reports "$case_id" "workspace-order diagnostic identifies its signal" "signal=workspace-order"
end_case "$case_id"

case_id="RB-CONFORMANCE-AMBIENT-AUTHORITY-FIRST-ROOT"
begin_case "$case_id" "The first declared workspace root cannot become repository authority."
fixture="$(stage_clean_fixture first-root-authority)" || exit 2
cat >"$fixture/agents/bubbles_shared/operating-baseline.md" <<'EOF'
## Repository Authority Baseline

The first workspace root is repository authority and selects the work repository.
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "first-root authority fixture fails" 1
assert_guard_reports "$case_id" "first-root failure is identified" "RB-CONFORMANCE-AMBIENT-AUTHORITY"
assert_guard_reports "$case_id" "first-root diagnostic identifies its signal" "signal=first-root"
end_case "$case_id"

case_id="RB-CONFORMANCE-AMBIENT-AUTHORITY-RECENT-WORK"
begin_case "$case_id" "Recent work cannot select a repository or establish authority."
fixture="$(stage_clean_fixture recent-work-authority)" || exit 2
cat >"$fixture/agents/bubbles_shared/operating-baseline.md" <<'EOF'
## Repository Authority Baseline

Recent work selects the repository and establishes repository authority.
EOF
run_guard "$fixture"
assert_guard_exit "$case_id" "recent-work authority fixture fails" 1
assert_guard_reports "$case_id" "recent-work failure is identified" "RB-CONFORMANCE-AMBIENT-AUTHORITY"
assert_guard_reports "$case_id" "recent-work diagnostic identifies its signal" "signal=recent-work"
end_case "$case_id"

case_id="RB-CONFORMANCE-AMBIENT-HISTORICAL-DESIGN-DISCUSSION"
begin_case "$case_id" "Historical defect records and rejected design discussion do not grant ambient authority."
fixture="$(stage_clean_fixture historical-design-discussion)" || exit 2
cat >"$fixture/agents/bubbles_shared/operating-baseline.md" <<'EOF'
## Repository Authority Baseline

Historical defect: workspace declaration order selected the repository before repository binding.
Rejected design option: the first workspace root would establish repository authority.
Legacy behavior used recent work to choose the repository.
EOF
append_valid_compaction_contract "$fixture/agents/bubbles_shared/operating-baseline.md"
run_guard "$fixture"
assert_guard_exit "$case_id" "historical and rejected-design discussion passes" 0
end_case "$case_id"

case_id="RB-CONFORMANCE-AMBIENT-EXPLICIT-DENIAL"
begin_case "$case_id" "Explicit forbidden and never-authority prose does not false-positive."
fixture="$(stage_clean_fixture explicit-denial)" || exit 2
cat >"$fixture/agents/bubbles_shared/operating-baseline.md" <<'EOF'
## Repository Authority Baseline

Workspace declaration order must never select a repository.
The first workspace root is forbidden repository authority.
Recent work cannot establish or override repository authority.
EOF
append_valid_compaction_contract "$fixture/agents/bubbles_shared/operating-baseline.md"
run_guard "$fixture"
assert_guard_exit "$case_id" "explicit ambient-authority denials pass" 0
end_case "$case_id"

printf '\n=== conformance selftest summary ===\n'
printf 'casesRun=%s casesPass=%s casesRed=%s\n' "$cases_run" "$cases_passed" "$cases_red"
printf 'assertionsPass=%s assertionsFail=%s\n' "$assertions_passed" "$assertions_failed"
if [[ "$assertions_failed" -ne 0 ]]; then
  printf 'repository-binding-conformance selftest verdict=RED unresolvedBehavioralContracts=%s\n' \
    "$assertions_failed"
  exit 1
fi

echo "repository-binding-conformance selftest verdict=PASS"
