#!/usr/bin/env bash
#
# eval-harness-selftest.sh — hermetic fail-closed evaluation regressions.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
HARNESS="$SCRIPT_DIR/eval-harness.sh"
TASKS="$REPO_ROOT/bubbles/eval/tasks"
FIXTURES="$REPO_ROOT/bubbles/eval/fixtures"
BASH_BIN="$(command -v bash)"

if ! command -v python3 >/dev/null 2>&1; then
  echo "eval-harness-selftest: SKIP (python3 not installed; harness unavailability is covered when Python exists)"
  exit 0
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

pass_count=0
fail_count=0
pass() { printf '  PASS: %s\n' "$1"; pass_count=$((pass_count + 1)); }
fail() { printf '  FAIL: %s\n' "$1"; fail_count=$((fail_count + 1)); }

assert_exit() {
  local actual="$1"
  local expected="$2"
  local label="$3"
  if [[ "$actual" -eq "$expected" ]]; then
    pass "$label (exit $actual)"
  else
    fail "$label (expected exit $expected, observed $actual)"
  fi
}

assert_field() {
  local result_file="$1"
  local path="$2"
  local expected_json="$3"
  local label="$4"
  if python3 - "$result_file" "$path" "$expected_json" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    value = json.load(handle)
for component in sys.argv[2].split("."):
    value = value[int(component)] if isinstance(value, list) else value[component]
expected = json.loads(sys.argv[3])
raise SystemExit(0 if value == expected else 1)
PY
  then
    pass "$label"
  else
    fail "$label (field $path did not equal $expected_json)"
  fi
}

assert_code() {
  local result_file="$1"
  local expected_code="$2"
  local label="$3"
  if python3 - "$result_file" "$expected_code" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    payload = json.load(handle)

def contains_code(value, expected):
    if isinstance(value, dict):
        if value.get("code") == expected:
            return True
        return any(contains_code(item, expected) for item in value.values())
    if isinstance(value, list):
        return any(contains_code(item, expected) for item in value)
    return False

raise SystemExit(0 if contains_code(payload, sys.argv[2]) else 1)
PY
  then
    pass "$label"
  else
    fail "$label (missing code $expected_code)"
  fi
}

assert_check() {
  local result_file="$1"
  local check_id="$2"
  local expected_status="$3"
  local expected_code="$4"
  local label="$5"
  if python3 - "$result_file" "$check_id" "$expected_status" "$expected_code" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    payload = json.load(handle)
matches = [check for check in payload.get("checks", []) if check.get("id") == sys.argv[2]]
if len(matches) != 1 or matches[0].get("status") != sys.argv[3]:
    raise SystemExit(1)
expected_code = sys.argv[4]
observed_code = (matches[0].get("error") or {}).get("code")
raise SystemExit(0 if not expected_code or observed_code == expected_code else 1)
PY
  then
    pass "$label"
  else
    fail "$label (check $check_id was not $expected_status/$expected_code)"
  fi
}

assert_check_field() {
  local result_file="$1"
  local check_id="$2"
  local field_path="$3"
  local expected_json="$4"
  local label="$5"
  if python3 - "$result_file" "$check_id" "$field_path" "$expected_json" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    payload = json.load(handle)
matches = [check for check in payload.get("checks", []) if check.get("id") == sys.argv[2]]
if len(matches) != 1:
    raise SystemExit(1)
value = matches[0]
for component in sys.argv[3].split("."):
    value = value[int(component)] if isinstance(value, list) else value[component]
raise SystemExit(0 if value == json.loads(sys.argv[4]) else 1)
PY
  then
    pass "$label"
  else
    fail "$label (check $check_id field $field_path did not equal $expected_json)"
  fi
}

TASK_PACK="$TMPDIR/task-pack"
NEGATIVE_TASKS="$TMPDIR/negative-tasks"
ADAPTERS="$TMPDIR/adapters"
GOOD="$TMPDIR/good"
mkdir -p "$TASK_PACK/oracles" "$NEGATIVE_TASKS/oracles" "$ADAPTERS" "$GOOD"
cp "$TASKS/golden-bugfix-001.json" "$TASK_PACK/golden-bugfix-001.json"
cp "$TASKS/golden-feature-001.json" "$TASK_PACK/golden-feature-001.json"
cp "$FIXTURES/negative/tasks/"*.json "$NEGATIVE_TASKS/"
cp "$FIXTURES/positive/oracles/fixture-oracle.py" "$TASK_PACK/oracles/fixture-oracle.py"
cp "$FIXTURES/positive/oracles/fixture-oracle.py" "$NEGATIVE_TASKS/oracles/fixture-oracle.py"
cp "$FIXTURES/positive/oracles/fixture-oracle.py" "$NEGATIVE_TASKS/fixture-oracle.py"
cp "$FIXTURES/positive/adapters/evaluator-fixture.py" "$ADAPTERS/evaluator-fixture.py"
chmod +x \
  "$TASK_PACK/oracles/fixture-oracle.py" \
  "$NEGATIVE_TASKS/oracles/fixture-oracle.py" \
  "$NEGATIVE_TASKS/fixture-oracle.py" \
  "$ADAPTERS/evaluator-fixture.py"
cp -R "$FIXTURES/positive/bugfix-output/." "$GOOD/"
cp -R "$FIXTURES/positive/feature-output/." "$GOOD/"

BUGFIX_TASK="$TASK_PACK/golden-bugfix-001.json"
FEATURE_TASK="$TASK_PACK/golden-feature-001.json"
ADAPTER="$ADAPTERS/evaluator-fixture.py"

printf '%s\n' '[eval-harness-selftest] v2 substantive golden tasks'

bugfix_good="$TMPDIR/bugfix-good.json"
rc=0
"$BASH_BIN" "$HARNESS" score --task "$BUGFIX_TASK" --output "$GOOD" > "$bugfix_good" || rc=$?
assert_exit "$rc" 0 'bugfix behavioral positive passes'
assert_field "$bugfix_good" evaluationStatus '"passed"' 'bugfix positive reports passed'
assert_field "$bugfix_good" certified 'true' 'bugfix positive is certifying v2 output'
assert_field "$bugfix_good" taskSchemaVersion '2' 'bugfix positive uses schema v2'
assert_check "$bugfix_good" bugfix-end-state passed '' 'bugfix required executable oracle passes'
assert_check_field "$bugfix_good" bugfix-end-state execution.shell 'false' 'bugfix oracle records shell=false'

feature_good="$TMPDIR/feature-good.json"
rc=0
"$BASH_BIN" "$HARNESS" score --task "$FEATURE_TASK" --output "$GOOD" > "$feature_good" || rc=$?
assert_exit "$rc" 0 'feature behavioral positive passes'
assert_field "$feature_good" evaluationStatus '"passed"' 'feature positive reports passed'
assert_field "$feature_good" certified 'true' 'feature positive is certifying v2 output'
assert_check "$feature_good" feature-contract-end-state passed '' 'feature parser oracle proves coherent contract behavior'

hollow_result="$TMPDIR/hollow-report.json"
rc=0
"$BASH_BIN" "$HARNESS" score --task "$BUGFIX_TASK" --output "$FIXTURES/negative/hollow-report" > "$hollow_result" || rc=$?
assert_exit "$rc" 1 'exact hollow report is rejected'
assert_field "$hollow_result" evaluationStatus '"failed"' 'exact hollow report reports failed'
assert_field "$hollow_result" certified 'false' 'exact hollow report cannot certify'
assert_check "$hollow_result" bugfix-end-state failed oracle-nonzero 'exact hollow report fails required behavior oracle'

token_result="$TMPDIR/token-stuffed-spec.json"
rc=0
"$BASH_BIN" "$HARNESS" score --task "$FEATURE_TASK" --output "$FIXTURES/negative/token-stuffed-spec" > "$token_result" || rc=$?
assert_exit "$rc" 1 'one-line token-stuffed spec is rejected'
assert_field "$token_result" evaluationStatus '"failed"' 'token-stuffed spec reports failed'
assert_check "$token_result" feature-contract-end-state failed oracle-nonzero 'token-stuffed spec fails required parser oracle'

suite_result="$TMPDIR/suite-good.json"
rc=0
"$BASH_BIN" "$HARNESS" run --suite "$TASK_PACK" --output "$GOOD" > "$suite_result" || rc=$?
assert_exit "$rc" 0 'golden v2 suite aggregates behavioral positives'
assert_field "$suite_result" taskCount '2' 'golden suite runs both tasks'
assert_field "$suite_result" certified 'true' 'golden suite certifies only when both oracles pass'
assert_field "$suite_result" certifyingPassed '2' 'golden suite records two certifying passes'

printf '%s\n' '[eval-harness-selftest] task schema and check failures'

invalid_schema_result="$TMPDIR/invalid-schema.json"
rc=0
"$BASH_BIN" "$HARNESS" score --task "$NEGATIVE_TASKS/invalid-schema.json" --output "$GOOD" > "$invalid_schema_result" || rc=$?
assert_exit "$rc" 2 'invalid task schema fails before scoring'
assert_field "$invalid_schema_result" inputValid 'false' 'invalid task schema reports inputValid=false'
assert_code "$invalid_schema_result" task-schema-invalid 'invalid task schema emits task-schema-invalid'

malformed_task_result="$TMPDIR/malformed-task.json"
rc=0
"$BASH_BIN" "$HARNESS" score --task "$NEGATIVE_TASKS/malformed-task.json" --output "$GOOD" > "$malformed_task_result" || rc=$?
assert_exit "$rc" 2 'malformed task JSON fails before scoring'
assert_field "$malformed_task_result" evaluationStatus '"error"' 'malformed task JSON reports error'
assert_code "$malformed_task_result" task-schema-invalid 'malformed task JSON emits task-schema-invalid'

unknown_required_result="$TMPDIR/unknown-required.json"
rc=0
"$BASH_BIN" "$HARNESS" score --task "$NEGATIVE_TASKS/unknown-required.json" --output "$GOOD" > "$unknown_required_result" || rc=$?
assert_exit "$rc" 2 'unknown required check invalidates task'
assert_code "$unknown_required_result" unknown-required-check 'unknown required check reason is explicit'

unknown_optional_result="$TMPDIR/unknown-optional.json"
rc=0
"$BASH_BIN" "$HARNESS" score --task "$NEGATIVE_TASKS/unknown-optional.json" --output "$GOOD" > "$unknown_optional_result" || rc=$?
assert_exit "$rc" 1 'weighted unknown optional check cannot silently certify'
assert_field "$unknown_optional_result" inputValid 'true' 'unknown optional check remains schema-valid'
assert_field "$unknown_optional_result" evaluationStatus '"unavailable"' 'weighted unknown optional check retains unavailable status'
assert_check "$unknown_optional_result" unknown-optional unavailable unknown-optional-check 'unknown optional check is visibly unavailable'

missing_evaluator_result="$TMPDIR/missing-required-evaluator.json"
rc=0
"$BASH_BIN" "$HARNESS" score --task "$NEGATIVE_TASKS/missing-required-evaluator.json" --output "$FIXTURES/negative/hollow-report" > "$missing_evaluator_result" || rc=$?
assert_exit "$rc" 2 'task without required substantive evaluator is invalid'
assert_code "$missing_evaluator_result" substantive-check-required 'missing required evaluator reason is explicit'

missing_oracle_result="$TMPDIR/missing-oracle.json"
rc=0
"$BASH_BIN" "$HARNESS" score --task "$NEGATIVE_TASKS/missing-oracle.json" --output "$GOOD" > "$missing_oracle_result" || rc=$?
assert_exit "$rc" 1 'missing required oracle fails closed'
assert_field "$missing_oracle_result" evaluationStatus '"unavailable"' 'missing oracle reports unavailable'
assert_check "$missing_oracle_result" missing-oracle unavailable oracle-executable-unavailable 'missing oracle error is machine-readable'

oracle_nonzero_result="$TMPDIR/oracle-nonzero.json"
rc=0
"$BASH_BIN" "$HARNESS" score --task "$NEGATIVE_TASKS/oracle-nonzero.json" --output "$GOOD" > "$oracle_nonzero_result" || rc=$?
assert_exit "$rc" 1 'oracle nonzero fails required check'
assert_field "$oracle_nonzero_result" evaluationStatus '"failed"' 'oracle nonzero reports failed'
assert_check "$oracle_nonzero_result" nonzero-oracle failed oracle-nonzero 'oracle nonzero preserves exit failure code'

oracle_timeout_result="$TMPDIR/oracle-timeout.json"
rc=0
"$BASH_BIN" "$HARNESS" score --task "$NEGATIVE_TASKS/oracle-timeout.json" --output "$GOOD" > "$oracle_timeout_result" || rc=$?
assert_exit "$rc" 1 'oracle timeout fails closed'
assert_field "$oracle_timeout_result" evaluationStatus '"error"' 'oracle timeout reports error'
assert_check "$oracle_timeout_result" timeout-oracle error oracle-timeout 'oracle timeout reason is explicit'

metachar_result="$TMPDIR/oracle-metacharacters.json"
rc=0
"$BASH_BIN" "$HARNESS" score --task "$NEGATIVE_TASKS/oracle-metacharacters.json" --output "$GOOD" > "$metachar_result" || rc=$?
assert_exit "$rc" 0 'shell metacharacters remain literal argv values'
assert_check "$metachar_result" literal-argv passed '' 'literal argv oracle observes exact contained arguments'
assert_check_field "$metachar_result" literal-argv execution.shell 'false' 'metacharacter oracle records shell=false'
if [[ ! -e "$GOOD/injected" && ! -e "$GOOD/touch" ]]; then
  pass 'metacharacter argv creates no injected side effect'
else
  fail 'metacharacter argv escaped containment and created a side effect'
fi

outside_root_result="$TMPDIR/oracle-outside-root.json"
rc=0
"$BASH_BIN" "$HARNESS" score --task "$NEGATIVE_TASKS/oracle-outside-root.json" --output "$GOOD" > "$outside_root_result" || rc=$?
assert_exit "$rc" 1 'oracle executable outside allowlisted root is rejected'
assert_field "$outside_root_result" evaluationStatus '"error"' 'outside-root oracle reports error'
assert_check "$outside_root_result" outside-root error oracle-executable-outside-root 'outside-root oracle has containment error code'

printf '%s\n' '[eval-harness-selftest] semantic evaluator contract'

semantic_missing_result="$TMPDIR/semantic-missing.json"
rc=0
BUBBLES_EVAL_SEMANTIC="" "$BASH_BIN" "$HARNESS" score --task "$NEGATIVE_TASKS/semantic-required.json" --output "$GOOD" > "$semantic_missing_result" || rc=$?
assert_exit "$rc" 1 'missing required semantic evaluator fails closed'
assert_field "$semantic_missing_result" evaluationStatus '"unavailable"' 'missing semantic evaluator reports unavailable'
assert_check "$semantic_missing_result" required-semantic unavailable semantic-adapter-missing 'missing semantic evaluator reason is explicit'

semantic_success_result="$TMPDIR/semantic-success.json"
rc=0
EVAL_FIXTURE_MODE=pass BUBBLES_EVAL_SEMANTIC="$ADAPTER" "$BASH_BIN" "$HARNESS" score --task "$NEGATIVE_TASKS/semantic-required.json" --output "$GOOD" > "$semantic_success_result" || rc=$?
assert_exit "$rc" 0 'deterministic semantic evaluator passes real end-state behavior'
assert_field "$semantic_success_result" evaluationStatus '"passed"' 'semantic success reports passed'
assert_check "$semantic_success_result" required-semantic passed '' 'semantic success records required check pass'
assert_check_field "$semantic_success_result" required-semantic provenance.adapter '"deterministic-fixture-evaluator"' 'semantic success records provenance'

printf '%s\n' '[eval-harness-selftest] weighted judge contract'

judge_task="$NEGATIVE_TASKS/judge-required.json"
judge_success_result="$TMPDIR/judge-success.json"
rc=0
EVAL_FIXTURE_MODE=pass BUBBLES_EVAL_JUDGE="$ADAPTER" "$BASH_BIN" "$HARNESS" score --task "$judge_task" --output "$GOOD" > "$judge_success_result" || rc=$?
assert_exit "$rc" 0 'weighted judge passes only with valid behavioral result'
assert_field "$judge_success_result" judge.status '"passed"' 'successful judge status is present'
assert_field "$judge_success_result" judge.provenance.adapter '"deterministic-fixture-evaluator"' 'successful judge provenance is present'

judge_missing_result="$TMPDIR/judge-missing.json"
rc=0
BUBBLES_EVAL_JUDGE="" "$BASH_BIN" "$HARNESS" score --task "$judge_task" --output "$GOOD" > "$judge_missing_result" || rc=$?
assert_exit "$rc" 1 'missing weighted judge fails closed'
assert_field "$judge_missing_result" judge.status '"unavailable"' 'missing judge status remains visible'
assert_code "$judge_missing_result" judge-adapter-missing 'missing judge reason is explicit'

judge_printf_result="$TMPDIR/judge-printf.json"
rc=0
BUBBLES_EVAL_JUDGE=/usr/bin/printf "$BASH_BIN" "$HARNESS" score --task "$judge_task" --output "$GOOD" > "$judge_printf_result" || rc=$?
assert_exit "$rc" 1 'exact /usr/bin/printf hollow judge output fails closed'
assert_field "$judge_printf_result" evaluationStatus '"error"' 'invalid printf judge output reports evaluation error'
assert_field "$judge_printf_result" judge.status '"error"' 'invalid printf judge attempt cannot disappear'
assert_code "$judge_printf_result" judge-nonzero 'invalid printf judge output has nonzero adapter code'

for judge_mode in nonzero timeout invalid-output malformed-json missing-provenance nan infinity out-of-range; do
  judge_result="$TMPDIR/judge-$judge_mode.json"
  active_judge_task="$judge_task"
  case "$judge_mode" in
    nonzero) expected_code=judge-nonzero ;;
    timeout)
      expected_code=judge-timeout
      active_judge_task="$NEGATIVE_TASKS/judge-timeout.json"
      ;;
    invalid-output) expected_code=judge-schema-invalid ;;
    malformed-json|nan|infinity) expected_code=judge-malformed-json ;;
    missing-provenance|out-of-range) expected_code=judge-schema-invalid ;;
    *) expected_code=unexpected ;;
  esac
  rc=0
  EVAL_FIXTURE_MODE="$judge_mode" BUBBLES_EVAL_JUDGE="$ADAPTER" "$BASH_BIN" "$HARNESS" score --task "$active_judge_task" --output "$GOOD" > "$judge_result" || rc=$?
  assert_exit "$rc" 1 "judge $judge_mode fails closed"
  assert_field "$judge_result" evaluationStatus '"error"' "judge $judge_mode reports evaluation error"
  assert_field "$judge_result" judge.status '"error"' "judge $judge_mode attempt remains visible"
  assert_code "$judge_result" "$expected_code" "judge $judge_mode emits $expected_code"
done

printf '%s\n' '[eval-harness-selftest] required runtime failure'

no_python_path="$TMPDIR/no-python-path"
mkdir -p "$no_python_path"
python_missing_result="$TMPDIR/python-missing.json"
rc=0
PATH="$no_python_path" "$BASH_BIN" "$HARNESS" score --task "$BUGFIX_TASK" --output "$GOOD" > "$python_missing_result" || rc=$?
assert_exit "$rc" 1 'missing Python runtime is unavailable, never a quality pass'
assert_field "$python_missing_result" evaluationStatus '"unavailable"' 'missing Python runtime reports unavailable'
assert_field "$python_missing_result" certified 'false' 'missing Python runtime cannot certify'
assert_code "$python_missing_result" python3-unavailable 'missing Python runtime reason is explicit'

printf '\n[eval-harness-selftest] %s passed, %s failed\n' "$pass_count" "$fail_count"
[[ "$fail_count" -eq 0 ]] || exit 1
printf '%s\n' '[eval-harness-selftest] OK'
exit 0
