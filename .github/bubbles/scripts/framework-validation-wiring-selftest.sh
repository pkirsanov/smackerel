#!/usr/bin/env bash
# Focused framework-validation wiring contract for canonical scenario checks.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VALIDATOR="$SCRIPT_DIR/framework-validate.sh"
READER_SELFTEST="scenario-reference-reader-selftest.sh"
GENERATOR="$SCRIPT_DIR/generate-validation-checks.sh"
CONFIG_CONTRACT="$SCRIPT_DIR/../../agents/bubbles_shared/project-config-contract.md"

failures=0
pass() { printf 'PASS: %s\n' "$1"; }
fail() {
  printf 'FAIL: %s\n' "$1"
  failures=$((failures + 1))
}

if [[ ! -r "$VALIDATOR" || ! -r "$GENERATOR" ]]; then
  printf 'framework-validation-wiring-selftest: required validator/generator is not readable\n' >&2
  exit 2
fi

# Parse scheduled selftests with the same helper used by framework-validate's
# discovery sweep. Comments and incidental string mentions therefore cannot
# masquerade as an executable registration.
# shellcheck source=guard-lib.sh
source "$SCRIPT_DIR/guard-lib.sh"
validator_source="$(<"$VALIDATOR")"
scheduled="$(bubbles_scheduled_selftests "$validator_source")"
reader_count=0
while IFS= read -r scheduled_name; do
  [[ "$scheduled_name" == "$READER_SELFTEST" ]] || continue
  reader_count=$((reader_count + 1))
done <<<"$scheduled"

if [[ "$reader_count" -eq 1 ]]; then
  pass "reader selftest is scheduled exactly once"
else
  fail "reader selftest must be scheduled exactly once (observed $reader_count)"
fi

expected_registration='run_check "Scenario reference reader selftest (IMP-040 / COV-8)" bash "$SCRIPT_DIR/scenario-reference-reader-selftest.sh"'
if grep -Fxq "$expected_registration" "$VALIDATOR"; then
  pass "reader selftest uses the canonical unconditional run_check path"
else
  fail "reader selftest must use the canonical unconditional run_check path"
fi

while IFS='|' read -r selftest_name registration; do
  registration_count="$(grep -Fxc "$registration" "$VALIDATOR" || true)"
  if [[ "$registration_count" -eq 1 ]]; then
    pass "$selftest_name uses the canonical unconditional run_check path exactly once"
  else
    fail "$selftest_name must use the canonical unconditional run_check path exactly once (observed $registration_count)"
  fi
done <<'EOF'
scenario manifest v2 schema selftest|run_check "Scenario manifest v2 schema selftest (IMP-040 / COV-8)" bash "$SCRIPT_DIR/scenario-manifest-v2-schema-selftest.sh"
scenario manifest migration selftest|run_check "Scenario manifest migration selftest (IMP-040 / COV-8)" bash "$SCRIPT_DIR/scenario-manifest-migrate-selftest.sh"
YAML schema dispatch selftest|run_check "YAML schema dispatch selftest (IMP-040 / COV-8)" bash "$SCRIPT_DIR/yaml-schema-validate-selftest.sh"
framework validation wiring selftest|run_check "Framework validation wiring selftest (IMP-040 / COV-8)" bash "$SCRIPT_DIR/framework-validation-wiring-selftest.sh"
execution-control store selftest|run_check "Execution-control store selftest (IMP-054/055 / ECF-01)" bash "$SCRIPT_DIR/execution-control-selftest.sh"
measured-budget runtime selftest|run_check "Measured-budget runtime selftest (IMP-055 / MBE-01)" python3 "$SCRIPT_DIR/measured-budget-runtime-selftest.py"
research runtime selftest|run_check "Research runtime selftest (IMP-054 / RESEARCH-01)" python3 "$SCRIPT_DIR/research-runtime-selftest.py"
research adapter contract selftest|run_check "Research adapter contract selftest (IMP-054 / RESEARCH-02)" python3 "$SCRIPT_DIR/research-adapter-contract-selftest.py"
usage adapter v2 selftest|run_check "Usage adapter v2 selftest (IMP-055 / USAGE-02)" bash "$SCRIPT_DIR/usage-adapter-v2-selftest.sh"
admission contract selftest|run_check "Admission contract selftest (IMP-055 / ADMISSION-01)" bash "$SCRIPT_DIR/admission-contract-selftest.sh"
research and admission CLI integration selftest|run_check "Research and admission CLI integration selftest (IMP-054/055)" bash "$SCRIPT_DIR/research-admission-cli-selftest.sh"
EOF

research_direct_tests="$SCRIPT_DIR/../registry/research-direct-tests.json"
if [[ "$(jq -r '.sharedValidationStatus' "$research_direct_tests")" == "registered-unconditionally" ]] \
  && [[ "$(jq -r '.sharedValidationDriver' "$research_direct_tests")" == "bubbles/scripts/framework-validate.sh" ]]; then
  pass "research direct-test metadata records unconditional shared validation registration"
else
  fail "research direct-test metadata must record its canonical unconditional registration"
fi

resolver_line="$(grep -nFx 'run_check "Scenario linked-test resolution selftest (IMP-040 / COV-8)" bash "$SCRIPT_DIR/scenario-test-resolve-selftest.sh"' "$VALIDATOR" | cut -d: -f1)"
reader_line="$(grep -nFx "$expected_registration" "$VALIDATOR" | cut -d: -f1)"
if [[ -n "$resolver_line" && -n "$reader_line" ]] && (( reader_line == resolver_line + 1 )); then
  pass "reader selftest remains adjacent to scenario resolution"
else
  fail "reader selftest must remain immediately after scenario resolution"
fi

cr02_count=0
while IFS= read -r registration; do
  [[ "$registration" == *'run_check_self_only "CR-02 traceability current-scope universe regression"'* ]] || continue
  [[ "$registration" == *'tests/regression/test_33_traceability_current_scope_universe.sh'* ]] || continue
  cr02_count=$((cr02_count + 1))
done < <(awk '{ line=$0; sub(/^[ \t]+/, "", line); if (pending != "") { line=pending " " line; pending="" } if (line ~ /\\$/) { sub(/\\$/, "", line); pending=line; next } if (line ~ /^run_check(_self_only)?[ \t]/) print line }' "$VALIDATOR")
if [[ "$cr02_count" -eq 1 ]]; then
  pass "CR-02 regression is scheduled exactly once through run_check_self_only"
else
  fail "CR-02 regression must be scheduled exactly once through run_check_self_only (observed $cr02_count)"
fi

# Exercise the real validator and its production run_check/cache path in a
# bounded fixture. The focused probe exits before the normal schedule, so the
# registered wiring selftest cannot recursively invoke itself.
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT INT TERM
FIXTURE="$WORK/repo"
mkdir -p "$FIXTURE/bubbles/scripts" "$FIXTURE/bubbles/registry" "$FIXTURE/bubbles/schemas" \
  "$FIXTURE/bubbles/adapters/research" "$FIXTURE/bubbles/adapters/dispatch" \
  "$FIXTURE/bubbles/adapters/usage" "$FIXTURE/agents/bubbles_shared" "$FIXTURE/tests/regression"
cp "$VALIDATOR" "$FIXTURE/bubbles/scripts/framework-validate.sh"
cp "$SCRIPT_DIR/guard-lib.sh" "$FIXTURE/bubbles/scripts/guard-lib.sh"
cp "$SCRIPT_DIR/validate-cache.sh" "$FIXTURE/bubbles/scripts/validate-cache.sh"
cp "$SCRIPT_DIR/validation-closure.sh" "$FIXTURE/bubbles/scripts/validation-closure.sh"
cp "$GENERATOR" "$FIXTURE/bubbles/scripts/generate-validation-checks.sh"
cp "$SCRIPT_DIR/execution-control-selftest.sh" "$FIXTURE/bubbles/scripts/execution-control-selftest.sh"
for integration_file in \
  measured-budget-runtime-selftest.py measured-budget-runtime-v2-selftest.py \
  measured-budget-runtime.py measured-budget-contracts.py research-runtime-selftest.py \
  research-runtime.py research-run.sh research-adapter-contract-selftest.py \
  usage-adapter-v2-selftest.sh admission-contract-selftest.sh \
  research-admission-cli-selftest.sh cli.sh fun-mode.sh aliases.sh trust-metadata.sh \
  adoption-profile-lib.sh pre-tool-risk-gate.sh dispatch-admission.sh \
  goal-budget-ledger.sh session-epoch-authority.sh cost-corpus-evaluate.sh \
  dispatch-adapter-resolve.sh usage-resolve.sh; do
  cp "$SCRIPT_DIR/$integration_file" "$FIXTURE/bubbles/scripts/$integration_file"
done
for registry_file in research-runtime.json research-stages.yaml research-direct-tests.json \
  measured-budget-runtime-contracts.json admission-dimensions.yaml model-classes.yaml \
  execution-control-security-outcomes.json; do
  cp "$SCRIPT_DIR/../registry/$registry_file" "$FIXTURE/bubbles/registry/$registry_file"
done
for schema_file in dispatch-admission.schema.json session-epoch.schema.json usage-adapter-v2.schema.json; do
  cp "$SCRIPT_DIR/../schemas/$schema_file" "$FIXTURE/bubbles/schemas/$schema_file"
done
for adapter_file in disabled.sh local-command.sh; do
  cp "$SCRIPT_DIR/../adapters/research/$adapter_file" "$FIXTURE/bubbles/adapters/research/$adapter_file"
done
for adapter_file in none.sh reference-broker.sh; do
  cp "$SCRIPT_DIR/../adapters/dispatch/$adapter_file" "$FIXTURE/bubbles/adapters/dispatch/$adapter_file"
done
for adapter_file in none.sh vscode-copilot.sh reference-test.sh; do
  cp "$SCRIPT_DIR/../adapters/usage/$adapter_file" "$FIXTURE/bubbles/adapters/usage/$adapter_file"
done
cp "$SCRIPT_DIR/../action-risk-registry.yaml" "$FIXTURE/bubbles/action-risk-registry.yaml"
cp "$CONFIG_CONTRACT" "$FIXTURE/agents/bubbles_shared/project-config-contract.md"
cat >"$FIXTURE/bubbles/scripts/execution-control-v2-selftest.py" <<'EOF'
#!/usr/bin/env python3
print("execution-control fixture")
EOF
printf 'print("store fixture")\n' >"$FIXTURE/bubbles/scripts/execution-control-store.py"
printf 'print("lock fixture")\n' >"$FIXTURE/bubbles/scripts/execution-control-lock-selftest.py"
printf '{}\n' >"$FIXTURE/bubbles/schemas/execution-control-event.schema.json"
printf 'test-version\n' >"$FIXTURE/VERSION"

run_validator_probe() {
  (
    cd "$FIXTURE"
    BUBBLES_FRAMEWORK_VALIDATE_READER_PROBE=1 \
      BUBBLES_FRAMEWORK_VALIDATE_MODE=source \
      BUBBLES_VALIDATE_CACHE_DIR="$WORK/cache" \
      bash bubbles/scripts/framework-validate.sh "$@"
  )
}

run_cr02_probe() {
  (
    cd "$FIXTURE"
    BUBBLES_FRAMEWORK_VALIDATE_CR02_PROBE=1 \
      BUBBLES_FRAMEWORK_VALIDATE_MODE=source \
      BUBBLES_VALIDATE_CACHE_DIR="$WORK/cr02-cache" \
      bash bubbles/scripts/framework-validate.sh "$@"
  )
}

cat >"$FIXTURE/tests/regression/test_33_traceability_current_scope_universe.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'executed\n' >>"$BUBBLES_CR02_PROBE_COUNTER"
[[ "${BUBBLES_CR02_PROBE_FAIL:-0}" == "0" ]]
EOF
printf '' >"$WORK/cr02-executions"
export BUBBLES_CR02_PROBE_COUNTER="$WORK/cr02-executions"

cr02_failure_status=0
cr02_failure_output="$(BUBBLES_CR02_PROBE_FAIL=1 run_cr02_probe --no-cache 2>&1)" || cr02_failure_status=$?
if [[ "$cr02_failure_status" -eq 1 ]] && [[ "$cr02_failure_output" == *"FAIL: CR-02 traceability current-scope universe regression"* ]]; then
  pass "failing CR-02 regression blocks through the real validator path"
else
  fail "failing CR-02 regression must block the real validator path (exit=$cr02_failure_status)"
fi
printf '' >"$WORK/cr02-executions"

missing_status=0
missing_output="$(run_validator_probe --no-cache 2>&1)" || missing_status=$?
if [[ "$missing_status" -eq 1 ]] && [[ "$missing_output" == *"FAIL: Scenario reference reader selftest"* ]]; then
  pass "missing reader selftest fails through the real validator run_check path"
else
  fail "missing reader selftest must fail the real validator path (exit=$missing_status)"
fi

cat >"$FIXTURE/bubbles/scripts/scenario-reference-reader-selftest.sh" <<'EOF'
#!/usr/bin/env bash
exit 23
EOF
failing_status=0
failing_output="$(run_validator_probe --no-cache 2>&1)" || failing_status=$?
if [[ "$failing_status" -eq 1 ]] && [[ "$failing_output" == *"FAIL: Scenario reference reader selftest"* ]]; then
  pass "failing reader selftest fails through the real validator run_check path"
else
  fail "failing reader selftest must fail the real validator path (exit=$failing_status)"
fi

cat >"$FIXTURE/bubbles/scripts/scenario-reference-reader.py" <<'EOF'
import os

with open(os.environ["BUBBLES_READER_PROBE_COUNTER"], "a", encoding="utf-8") as counter:
    counter.write("executed\n")
EOF
cat >"$FIXTURE/bubbles/scripts/scenario-reference-reader-selftest.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
python3 "$SCRIPT_DIR/scenario-reference-reader.py"
EOF
printf '' >"$WORK/executions"
export BUBBLES_READER_PROBE_COUNTER="$WORK/executions"

generator_status=0
generator_output="$(bash "$FIXTURE/bubbles/scripts/generate-validation-checks.sh" --repo-root "$FIXTURE" 2>&1)" || generator_status=$?
missing_closure_inputs=()
for expected_input in \
  'script: bubbles/scripts/scenario-reference-reader-selftest.sh' \
  '- bubbles/scripts/scenario-reference-reader.py' \
  'script: bubbles/scripts/execution-control-selftest.sh' \
  '- bubbles/scripts/execution-control-v2-selftest.py' \
  '- bubbles/scripts/execution-control-store.py' \
  '- bubbles/scripts/execution-control-lock-selftest.py' \
  '- bubbles/schemas/execution-control-event.schema.json' \
  '- bubbles/registry/execution-control-security-outcomes.json' \
  'script: bubbles/scripts/research-admission-cli-selftest.sh' \
  '- bubbles/scripts/research-runtime.py' \
  '- bubbles/scripts/measured-budget-runtime.py' \
  '- bubbles/registry/measured-budget-runtime-contracts.json' \
  '- bubbles/schemas/dispatch-admission.schema.json' \
  '- bubbles/adapters/research/local-command.sh' \
  '- agents/bubbles_shared/project-config-contract.md' \
  'script: tests/regression/test_33_traceability_current_scope_universe.sh'; do
  grep -Fq -- "$expected_input" "$FIXTURE/bubbles/registry/validation-checks.yaml" \
    || missing_closure_inputs+=("$expected_input")
done
if [[ "$generator_status" -eq 0 && "${#missing_closure_inputs[@]}" -eq 0 ]]; then
  pass "fixture registry derives reader, ECF, research/admission, and CR-02 closures"
else
  fail "fixture registry must derive reader, ECF, research/admission, and CR-02 closures (exit=$generator_status missing=${missing_closure_inputs[*]:-none} output=$generator_output)"
fi

ecf_check_id="$(
  bash "$FIXTURE/bubbles/scripts/validation-closure.sh" id-for \
    bubbles/scripts/execution-control-selftest.sh \
    --registry "$FIXTURE/bubbles/registry/validation-checks.yaml" \
    --repo-root "$FIXTURE"
)"
ecf_initial_digest="$(
  bash "$FIXTURE/bubbles/scripts/validation-closure.sh" digest "$ecf_check_id" \
    --registry "$FIXTURE/bubbles/registry/validation-checks.yaml" \
    --repo-root "$FIXTURE"
)"
printf '\n# changed ECF driver\n' >>"$FIXTURE/bubbles/scripts/execution-control-v2-selftest.py"
ecf_changed_digest="$(
  bash "$FIXTURE/bubbles/scripts/validation-closure.sh" digest "$ecf_check_id" \
    --registry "$FIXTURE/bubbles/registry/validation-checks.yaml" \
    --repo-root "$FIXTURE"
)"
rm "$FIXTURE/bubbles/scripts/execution-control-v2-selftest.py"
ecf_removed_digest="$(
  bash "$FIXTURE/bubbles/scripts/validation-closure.sh" digest "$ecf_check_id" \
    --registry "$FIXTURE/bubbles/registry/validation-checks.yaml" \
    --repo-root "$FIXTURE"
)"
if [[ -n "$ecf_check_id" && -n "$ecf_initial_digest" && -n "$ecf_changed_digest" && -n "$ecf_removed_digest" ]] \
  && [[ "$ecf_initial_digest" != "$ecf_changed_digest" ]] \
  && [[ "$ecf_changed_digest" != "$ecf_removed_digest" ]]; then
  pass "changing or removing the declared ECF Python driver invalidates its closure digest"
else
  fail "ECF Python driver changes and removal must invalidate the closure digest"
fi

RESEARCH_FIXTURE="$WORK/research-closure"
mkdir -p "$RESEARCH_FIXTURE/bubbles/scripts" "$RESEARCH_FIXTURE/bubbles/registry"
cp "$GENERATOR" "$RESEARCH_FIXTURE/bubbles/scripts/generate-validation-checks.sh"
cp "$SCRIPT_DIR/validation-closure.sh" "$RESEARCH_FIXTURE/bubbles/scripts/validation-closure.sh"
cp "$SCRIPT_DIR/research-runtime.py" "$RESEARCH_FIXTURE/bubbles/scripts/research-runtime.py"
cp "$SCRIPT_DIR/../registry/research-runtime.json" "$RESEARCH_FIXTURE/bubbles/registry/research-runtime.json"
cat >"$RESEARCH_FIXTURE/bubbles/scripts/research-closure-probe.sh" <<'EOF'
#!/usr/bin/env bash
ENGINE="$SCRIPT_DIR/research-runtime.py"
REGISTRY="$SCRIPT_DIR/../registry/research-runtime.json"
[[ -r "$ENGINE" && -r "$REGISTRY" ]]
EOF
cat >"$RESEARCH_FIXTURE/bubbles/scripts/framework-validate.sh" <<'EOF'
#!/usr/bin/env bash
run_check "Research closure probe" bash "$SCRIPT_DIR/research-closure-probe.sh"
EOF
research_generator_output="$(
  bash "$RESEARCH_FIXTURE/bubbles/scripts/generate-validation-checks.sh" --repo-root "$RESEARCH_FIXTURE"
)"
research_cli_check_id="$(
  bash "$RESEARCH_FIXTURE/bubbles/scripts/validation-closure.sh" id-for \
    bubbles/scripts/research-closure-probe.sh \
    --registry "$RESEARCH_FIXTURE/bubbles/registry/validation-checks.yaml" \
    --repo-root "$RESEARCH_FIXTURE"
)"
research_initial_digest="$(
  bash "$RESEARCH_FIXTURE/bubbles/scripts/validation-closure.sh" digest "$research_cli_check_id" \
    --registry "$RESEARCH_FIXTURE/bubbles/registry/validation-checks.yaml" \
    --repo-root "$RESEARCH_FIXTURE"
)"
printf '\n# changed research engine\n' >>"$RESEARCH_FIXTURE/bubbles/scripts/research-runtime.py"
research_engine_digest="$(
  bash "$RESEARCH_FIXTURE/bubbles/scripts/validation-closure.sh" digest "$research_cli_check_id" \
    --registry "$RESEARCH_FIXTURE/bubbles/registry/validation-checks.yaml" \
    --repo-root "$RESEARCH_FIXTURE"
)"
printf '\n' >>"$RESEARCH_FIXTURE/bubbles/registry/research-runtime.json"
research_registry_digest="$(
  bash "$RESEARCH_FIXTURE/bubbles/scripts/validation-closure.sh" digest "$research_cli_check_id" \
    --registry "$RESEARCH_FIXTURE/bubbles/registry/validation-checks.yaml" \
    --repo-root "$RESEARCH_FIXTURE"
)"
if [[ -n "$research_cli_check_id" && -n "$research_initial_digest" && -n "$research_engine_digest" && -n "$research_registry_digest" ]] \
  && [[ "$research_initial_digest" != "$research_engine_digest" ]] \
  && [[ "$research_engine_digest" != "$research_registry_digest" ]]; then
  pass "research engine and registry changes invalidate their declared closure digest"
else
  fail "research engine and registry changes must invalidate their declared closure digest (generator=$research_generator_output)"
fi

cr02_cold_output="$(run_cr02_probe --cache 2>&1)"
cr02_cold_status=$?
if [[ "$cr02_cold_status" -eq 0 ]] && [[ "$cr02_cold_output" == *"PASS: CR-02 traceability current-scope universe regression"* ]] \
  && [[ "$(wc -l <"$WORK/cr02-executions" | tr -d ' ')" -eq 1 ]]; then
  pass "CR-02 cache cold miss executes the regression exactly once"
else
  fail "CR-02 cache cold miss must execute exactly once (exit=$cr02_cold_status)"
fi

cr02_reuse_output="$(run_cr02_probe --cache 2>&1)"
cr02_reuse_status=$?
if [[ "$cr02_reuse_status" -eq 0 ]] && [[ "$cr02_reuse_output" == *"REUSED: CR-02 traceability current-scope universe regression"* ]] \
  && [[ "$(wc -l <"$WORK/cr02-executions" | tr -d ' ')" -eq 1 ]]; then
  pass "unchanged CR-02 registry closure reuses the cached result"
else
  fail "unchanged CR-02 closure must reuse without execution (exit=$cr02_reuse_status)"
fi

printf '\n# CR-02 cache invalidation\n' >>"$FIXTURE/tests/regression/test_33_traceability_current_scope_universe.sh"
cr02_change_output="$(run_cr02_probe --cache 2>&1)"
cr02_change_status=$?
if [[ "$cr02_change_status" -eq 0 ]] && [[ "$cr02_change_output" == *"PASS: CR-02 traceability current-scope universe regression"* ]] \
  && [[ "$(wc -l <"$WORK/cr02-executions" | tr -d ' ')" -eq 2 ]]; then
  pass "changing the CR-02 regression invalidates cached validation"
else
  fail "CR-02 regression change must invalidate cache (exit=$cr02_change_status)"
fi

cold_output="$(run_validator_probe --cache 2>&1)"
cold_status=$?
if [[ "$cold_status" -eq 0 ]] && [[ "$cold_output" == *"PASS: Scenario reference reader selftest"* ]] \
  && [[ "$(wc -l <"$WORK/executions" | tr -d ' ')" -eq 1 ]]; then
  pass "cache cold miss executes the reader selftest once and records PASS"
else
  fail "cache cold miss must execute exactly once (exit=$cold_status)"
fi

reuse_output="$(run_validator_probe --cache 2>&1)"
reuse_status=$?
if [[ "$reuse_status" -eq 0 ]] && [[ "$reuse_output" == *"REUSED: Scenario reference reader selftest"* ]] \
  && [[ "$(wc -l <"$WORK/executions" | tr -d ' ')" -eq 1 ]]; then
  pass "unchanged declared closure reuses the cached reader result without duplicate execution"
else
  fail "unchanged reader closure must reuse exactly once (exit=$reuse_status)"
fi

printf '\n# reader selftest cache invalidation\n' >>"$FIXTURE/bubbles/scripts/scenario-reference-reader-selftest.sh"
selftest_change_output="$(run_validator_probe --cache 2>&1)"
selftest_change_status=$?
if [[ "$selftest_change_status" -eq 0 ]] && [[ "$selftest_change_output" == *"PASS: Scenario reference reader selftest"* ]] \
  && [[ "$(wc -l <"$WORK/executions" | tr -d ' ')" -eq 2 ]]; then
  pass "changing the reader selftest invalidates cached validation"
else
  fail "reader selftest change must invalidate cache (exit=$selftest_change_status)"
fi

printf '\n# declared implementation input cache invalidation\n' >>"$FIXTURE/bubbles/scripts/scenario-reference-reader.py"
input_change_output="$(run_validator_probe --cache 2>&1)"
input_change_status=$?
if [[ "$input_change_status" -eq 0 ]] && [[ "$input_change_output" == *"PASS: Scenario reference reader selftest"* ]] \
  && [[ "$(wc -l <"$WORK/executions" | tr -d ' ')" -eq 3 ]]; then
  pass "changing the declared reader implementation invalidates cached validation"
else
  fail "declared reader implementation change must invalidate cache (exit=$input_change_status)"
fi

if [[ "$failures" -ne 0 ]]; then
  printf 'framework-validation-wiring-selftest: %s failure(s)\n' "$failures" >&2
  exit 1
fi

printf 'framework-validation-wiring-selftest: all checks passed\n'