#!/usr/bin/env bash
set -euo pipefail

# Hermetic selftest for Gate G091 - planning_workflow_chain_gate.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/planning-workflow-chain-guard.sh"

if [[ ! -x "$GUARD" ]]; then
  echo "planning-workflow-chain-guard-selftest: guard not executable at $GUARD" >&2
  exit 2
fi

WORKSPACE="$(mktemp -d -t bubbles-g091-selftest-XXXXXXXX)"
cleanup() {
  rm -rf "$WORKSPACE" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

PASS_COUNT=0
FAIL_COUNT=0
FAILED_SCENARIOS=()

ok() { PASS_COUNT=$((PASS_COUNT + 1)); printf '  PASS: %s\n' "$*"; }
ko() { FAIL_COUNT=$((FAIL_COUNT + 1)); FAILED_SCENARIOS+=("$*"); printf '  FAIL: %s\n' "$*"; }

stage_repo() {
  local sid="$1"
  local repo="$WORKSPACE/$sid"
  rm -rf "$repo"
  mkdir -p "$repo/.specify/memory" "$repo/bubbles" "$repo/agents/bubbles_shared" "$repo/prompts"
  printf '%s' "$repo"
}

stage_downstream_repo() {
  local sid="$1"
  local repo="$WORKSPACE/$sid"
  rm -rf "$repo"
  mkdir -p "$repo/.specify/memory" "$repo/.github/bubbles" "$repo/.github/agents/bubbles_shared" "$repo/.github/prompts"
  printf '%s' "$repo"
}

write_prompts_clean() {
  local repo="$1"
  local prefix="${2:-}"
  local rel
  for rel in \
    agents/bubbles.goal.agent.md \
    agents/bubbles.workflow.agent.md \
    agents/bubbles.iterate.agent.md \
    agents/bubbles.sprint.agent.md \
    agents/bubbles_shared/workflow-orchestration-core.md \
    agents/bubbles_shared/workflow-input-bootstrap.md
  do
    mkdir -p "$(dirname "$repo/$prefix$rel")"
    cat > "$repo/$prefix$rel" <<'EOF'
# Clean planning-chain prompt fixture

When planning truth is missing or repaired, invoke the canonical chain:
bubbles.analyst -> bubbles.ux -> bubbles.design -> bubbles.plan.
UX is mandatory even for framework and operator work; non-UI UX defines workflow behavior, status language, blocked envelopes, and exception handling.
EOF
  done
}

write_workflows() {
  local repo="$1"
  local prefix="${2:-}"
  local missing_artifacts_action="${MISSING_ARTIFACTS_ACTION:-invoke bubbles.analyst -> bubbles.ux -> bubbles.design -> bubbles.plan -> continue current workflow}"
  local missing_design_action="${MISSING_DESIGN_ACTION:-invoke bubbles.analyst -> bubbles.ux -> bubbles.design -> bubbles.plan inline -> then proceed to implement}"
  local freshness_action="${FRESHNESS_ACTION:-invoke bubbles.analyst -> bubbles.ux -> bubbles.design -> bubbles.plan inline to reconcile active artifacts before continuing}"
  local value_bootstrap="${VALUE_BOOTSTRAP:-[ bubbles.analyst, bubbles.ux, bubbles.design, bubbles.plan ]}"
  local full_bootstrap="${FULL_BOOTSTRAP:-[ bubbles.analyst, bubbles.ux, bubbles.design, bubbles.plan ]}"
  local prelude="${PRELUDE_ANALYZE:-[ bubbles.analyst, bubbles.ux, bubbles.design, bubbles.plan ]}"
  local docs_mutation="${DOCS_PLANNING_TRUTH:-false}"

  mkdir -p "$repo/${prefix}bubbles"
  cat > "$repo/${prefix}bubbles/workflows.yaml" <<EOF
version: 1

gates:
  G091:
    name: planning_workflow_chain_gate
    description: BLOCKING gate fixture.

modeTemplates:
  base-delivery:
    statusCeiling: done
  delivery-quality-constraints:
    constraints:
      requireCanonicalPlanningChain: true
      planningChainAgents: [ bubbles.analyst, bubbles.ux, bubbles.design, bubbles.plan ]

autoEscalation:
  inlineActions:
    missingArtifacts:
      description: Required artifacts are missing
      action: $missing_artifacts_action
    missingDesignBeforeImplement:
      description: Design or scopes missing before implement
      action: $missing_design_action
    artifactFreshnessDrift:
      description: Active artifacts are stale
      action: $freshness_action

modes:
  value-first-e2e-batch:
    description: Delivery fixture
    statusCeiling: done
    constraints:
      allowBootstrapIterations: true
      bootstrapAgents: $value_bootstrap
      planningTruthMutation: true
  full-delivery:
    description: Delivery fixture
    inherits: [ base-delivery, delivery-quality-constraints ]
    constraints:
      allowBootstrapIterations: true
      bootstrapAgents: $full_bootstrap
      planningTruthMutation: true
      improvementPreludeProfiles:
        analyze-design-plan: $prelude
  docs-only:
    description: Docs exception fixture
    statusCeiling: docs_updated
    constraints:
      planningTruthMutation: $docs_mutation
  validate-only:
    description: Validate exception fixture
    statusCeiling: validated
    constraints:
      planningTruthMutation: false
  spec-review-only:
    description: Spec-review exception fixture
    statusCeiling: docs_updated
    constraints:
      planningTruthMutation: false
EOF

  unset MISSING_ARTIFACTS_ACTION MISSING_DESIGN_ACTION FRESHNESS_ACTION VALUE_BOOTSTRAP FULL_BOOTSTRAP PRELUDE_ANALYZE DOCS_PLANNING_TRUTH
}

run_guard() {
  local repo="$1"
  set +e
  bash "$GUARD" --root "$repo" > "$WORKSPACE/stdout.last" 2> "$WORKSPACE/stderr.last"
  local rc=$?
  set -e
  echo "$rc" > "$WORKSPACE/exit.last"
}

assert_exit() {
  local label="$1"
  local expected="$2"
  local actual
  actual="$(cat "$WORKSPACE/exit.last")"
  if [[ "$actual" -eq "$expected" ]]; then
    ok "$label exit=$actual"
  else
    ko "$label expected exit=$expected actual=$actual"
    cat "$WORKSPACE/stdout.last"
    cat "$WORKSPACE/stderr.last"
  fi
}

assert_stdout_contains() {
  local label="$1"
  local needle="$2"
  if grep -qF "$needle" "$WORKSPACE/stdout.last"; then
    ok "$label stdout contains '$needle'"
  else
    ko "$label stdout missing '$needle'"
    cat "$WORKSPACE/stdout.last"
  fi
}

assert_stderr_contains() {
  local label="$1"
  local needle="$2"
  if grep -qF "$needle" "$WORKSPACE/stderr.last"; then
    ok "$label stderr contains '$needle'"
  else
    ko "$label stderr missing '$needle'"
    cat "$WORKSPACE/stderr.last"
  fi
}

echo "=== planning-workflow-chain-guard-selftest (Gate G091) ==="

echo ""
echo "--- S1: missing artifact auto-escalation design-to-plan shortcut fails ---"
repo="$(stage_repo s1-missing-artifacts)"
write_prompts_clean "$repo"
MISSING_ARTIFACTS_ACTION="invoke bubbles.design -> bubbles.plan -> continue current workflow" write_workflows "$repo"
run_guard "$repo"
assert_exit "S1 missingArtifacts shortcut" 1
assert_stderr_contains "S1" "autoEscalation.inlineActions.missingArtifacts"
assert_stderr_contains "S1" "bubbles.analyst -> bubbles.ux -> bubbles.design -> bubbles.plan"

echo ""
echo "--- S2: delivery bootstrap missing analyst and UX fails ---"
repo="$(stage_repo s2-bootstrap-shortcut)"
write_prompts_clean "$repo"
FULL_BOOTSTRAP="[ bubbles.design, bubbles.plan ]" write_workflows "$repo"
run_guard "$repo"
assert_exit "S2 bootstrap shortcut" 1
assert_stderr_contains "S2" "G091"
assert_stderr_contains "S2" "modes.full-delivery.bootstrapAgents"
assert_stderr_contains "S2" "missing bubbles.analyst bubbles.ux"

echo ""
echo "--- S3: improvement prelude missing UX fails ---"
repo="$(stage_repo s3-prelude-no-ux)"
write_prompts_clean "$repo"
PRELUDE_ANALYZE="[ bubbles.analyst, bubbles.design, bubbles.plan ]" write_workflows "$repo"
run_guard "$repo"
assert_exit "S3 improvement prelude shortcut" 1
assert_stderr_contains "S3" "improvementPreludeProfiles.analyze-design-plan"
assert_stderr_contains "S3" "bubbles.ux"

echo ""
echo "--- S4: ordered canonical delivery planning chain passes ---"
repo="$(stage_repo s4-canonical)"
write_prompts_clean "$repo"
write_workflows "$repo"
run_guard "$repo"
assert_exit "S4 canonical chain" 0
assert_stdout_contains "S4" "PASS Gate G091"
assert_stdout_contains "S4" "ordered planning chain valid"

echo ""
echo "--- S4b: downstream .github-installed layout passes ---"
repo="$(stage_downstream_repo s4b-downstream-layout)"
write_prompts_clean "$repo" ".github/"
write_workflows "$repo" ".github/"
run_guard "$repo"
assert_exit "S4b downstream canonical chain" 0
assert_stdout_contains "S4b" "PASS Gate G091"
assert_stdout_contains "S4b" "ordered planning chain valid"

echo ""
echo "--- S5: docs/validate/spec-review exceptions require no planning mutation ---"
repo="$(stage_repo s5-exception-clean)"
write_prompts_clean "$repo"
write_workflows "$repo"
run_guard "$repo"
assert_exit "S5 clean exception" 0
assert_stdout_contains "S5" "PASS Gate G091"

repo="$(stage_repo s5-exception-mutates)"
write_prompts_clean "$repo"
DOCS_PLANNING_TRUTH="true" write_workflows "$repo"
run_guard "$repo"
assert_exit "S5 mutating docs exception" 1
assert_stderr_contains "S5" "modes.docs-only"
assert_stderr_contains "S5" "planningTruthMutation: true"

echo ""
echo "--- S6: misordered planning chain fails ---"
repo="$(stage_repo s6-misordered)"
write_prompts_clean "$repo"
FULL_BOOTSTRAP="[ bubbles.analyst, bubbles.design, bubbles.ux, bubbles.plan ]" write_workflows "$repo"
run_guard "$repo"
assert_exit "S6 misordered chain" 1
assert_stderr_contains "S6" "bubbles.ux must run before bubbles.design"

echo ""
echo "--- S7: prompt fallback shortcut fails unless marked FORBIDDEN ---"
repo="$(stage_repo s7-prompt-shortcut)"
write_prompts_clean "$repo"
write_workflows "$repo"
cat >> "$repo/agents/bubbles.goal.agent.md" <<'EOF'
Fallback: invoke bubbles.design and bubbles.plan when artifacts are missing.
EOF
run_guard "$repo"
assert_exit "S7 active prompt shortcut" 1
assert_stderr_contains "S7" "agents/bubbles.goal.agent.md"
assert_stderr_contains "S7" "design-to-plan shortcut"

repo="$(stage_repo s7-forbidden-example)"
write_prompts_clean "$repo"
write_workflows "$repo"
cat >> "$repo/agents/bubbles.goal.agent.md" <<'EOF'
FORBIDDEN example: invoke bubbles.design and bubbles.plan when artifacts are missing.
EOF
run_guard "$repo"
assert_exit "S7 forbidden example" 0
assert_stdout_contains "S7" "PASS Gate G091"

echo ""
echo "=== Selftest verdict ==="
printf '  Total assertions: %d\n' "$((PASS_COUNT + FAIL_COUNT))"
printf '  Passed:           %d\n' "$PASS_COUNT"
printf '  Failed:           %d\n' "$FAIL_COUNT"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
  echo "planning-workflow-chain-guard-selftest: FAILED" >&2
  for scenario in "${FAILED_SCENARIOS[@]}"; do
    echo "  - $scenario" >&2
  done
  exit 1
fi

echo "planning-workflow-chain-guard-selftest: PASSED"
exit 0