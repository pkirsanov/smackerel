#!/usr/bin/env bash
# Focused CLI/default-off/risk integration coverage for IMP-054 and IMP-055.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=guard-lib.sh
source "$SCRIPT_DIR/guard-lib.sh"
CLI="$SCRIPT_DIR/cli.sh"
RISK_GATE="$SCRIPT_DIR/pre-tool-risk-gate.sh"
CONFIG_CONTRACT_RELATIVE="agents/bubbles_shared/project-config-contract.md"
CONFIG_CONTRACT="$SCRIPT_DIR/../../$CONFIG_CONTRACT_RELATIVE"

# These are executable closure inputs for the command families exercised here.
# Keeping the paths literal lets generate-validation-checks.sh invalidate this
# check whenever an engine, contract, registry, schema, or adapter changes.
DEPENDENCIES=(
  "$SCRIPT_DIR/research-run.sh"
  "$SCRIPT_DIR/research-runtime.py"
  "$SCRIPT_DIR/measured-budget-runtime.py"
  "$SCRIPT_DIR/measured-budget-contracts.py"
  "$SCRIPT_DIR/execution-control-store.py"
  "$SCRIPT_DIR/dispatch-admission.sh"
  "$SCRIPT_DIR/goal-budget-ledger.sh"
  "$SCRIPT_DIR/session-epoch-authority.sh"
  "$SCRIPT_DIR/cost-corpus-evaluate.sh"
  "$SCRIPT_DIR/dispatch-adapter-resolve.sh"
  "$SCRIPT_DIR/usage-resolve.sh"
  "$SCRIPT_DIR/../registry/research-runtime.json"
  "$SCRIPT_DIR/../registry/research-stages.yaml"
  "$SCRIPT_DIR/../registry/research-direct-tests.json"
  "$SCRIPT_DIR/../registry/measured-budget-runtime-contracts.json"
  "$SCRIPT_DIR/../registry/admission-dimensions.yaml"
  "$SCRIPT_DIR/../registry/model-classes.yaml"
  "$SCRIPT_DIR/../schemas/dispatch-admission.schema.json"
  "$SCRIPT_DIR/../schemas/session-epoch.schema.json"
  "$SCRIPT_DIR/../schemas/usage-adapter-v2.schema.json"
  "$SCRIPT_DIR/../adapters/research/disabled.sh"
  "$SCRIPT_DIR/../adapters/research/local-command.sh"
  "$SCRIPT_DIR/../adapters/dispatch/none.sh"
  "$SCRIPT_DIR/../adapters/dispatch/reference-broker.sh"
  "$SCRIPT_DIR/../adapters/usage/none.sh"
  "$SCRIPT_DIR/../adapters/usage/vscode-copilot.sh"
  "$SCRIPT_DIR/../adapters/usage/reference-test.sh"
)

pass_count=0
fail_count=0
pass() { pass_count=$((pass_count + 1)); printf 'PASS: %s\n' "$1"; }
fail() { fail_count=$((fail_count + 1)); printf 'FAIL: %s\n' "$1"; }

assert_contains() {
  local label="$1" value="$2" expected="$3"
  if [[ "$value" == *"$expected"* ]]; then pass "$label"; else fail "$label (missing: $expected)"; fi
}

for dependency in "${DEPENDENCIES[@]}"; do
  if [[ -r "$dependency" ]]; then
    pass "declared integration dependency is readable: ${dependency#"$SCRIPT_DIR/../"}"
  else
    fail "declared integration dependency is unreadable: $dependency"
  fi
done

help_output="$(bubbles_run_with_timeout 20 bash "$CLI" help 2>&1)"
assert_contains "CLI help advertises research" "$help_output" "research <subcommand>"
assert_contains "CLI help advertises admission" "$help_output" "admission <subcommand>"

research_status="$(bubbles_run_with_timeout 20 bash "$CLI" research status 2>&1)"
assert_contains "research status delegates to the runtime check contract" "$research_status" '"contractType":"research-check"'
assert_contains "research status reports the real runtime verdict" "$research_status" '"status":"valid"'

research_capabilities="$(bubbles_run_with_timeout 20 bash "$CLI" research capabilities 2>&1)"
assert_contains "research is default-off without project activation" "$research_capabilities" '"enabled":false'
assert_contains "research does not invent hosted routes" "$research_capabilities" '"hostedRoutes":[]'
assert_contains "research activation remains parked" "$research_capabilities" '"parkedActivation":true'

admission_adapter="$(bubbles_run_with_timeout 20 bash "$CLI" admission adapter --names-only 2>&1)"
[[ "$admission_adapter" == *'adapter=none'* ]] && pass "dispatch admission defaults to none" || fail "dispatch admission default is not none"
usage_adapter="$(bubbles_run_with_timeout 20 bash "$CLI" admission usage --names-only 2>&1)"
[[ "$usage_adapter" == *'adapter=none'* ]] && pass "usage observation defaults to none" || fail "usage observation default is not none"

host_status=0
host_output="$(bubbles_run_with_timeout 20 bash "$CLI" admission host-enforce 2>&1)" || host_status=$?
if [[ "$host_status" -ne 0 && "$host_output" == *'host-native enforcement is unavailable'* ]]; then
  pass "unsupported host-native enforcement fails loud"
else
  fail "unsupported host-native enforcement must fail loud (exit=$host_status)"
fi

while IFS='|' read -r label expected command args; do
  # shellcheck disable=SC2086 # The table intentionally supplies separate argv tokens.
  actual="$(bubbles_run_with_timeout 20 bash "$RISK_GATE" --resolve "$command" $args 2>&1)"
  if [[ "$actual" == "$expected" ]]; then pass "$label"; else fail "$label (expected=$expected actual=$actual)"; fi
done <<'EOF'
research status is read-only|read_only|research|status
research planning owns repository state|owned_mutation|research|plan --question fixture
research local command has an external side effect|external_side_effect|research|adapter-local-command --command true
admission adapter resolution is read-only|read_only|admission|adapter
admission usage resolution is read-only|read_only|admission|usage
admission budget snapshot is read-only|read_only|admission|budget snapshot fixture
admission reservation owns repository state|owned_mutation|admission|budget reserve fixture 1
EOF

contract_text="$(<"$CONFIG_CONTRACT")"
assert_contains "config contract documents dispatchAdmission" "$contract_text" 'dispatchAdmission:'
assert_contains "config contract documents researchRuntime" "$contract_text" 'researchRuntime:'
assert_contains "config contract documents explicit schema version" "$contract_text" 'schemaVersion: 1'
assert_contains "config contract documents neutral dispatch adapter" "$contract_text" 'adapter: none'

printf 'research-admission-cli-selftest: PASS=%s FAIL=%s\n' "$pass_count" "$fail_count"
[[ "$fail_count" -eq 0 ]]
