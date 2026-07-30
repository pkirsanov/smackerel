#!/usr/bin/env bash
# Gate-detection contract selftest.
# Contract: agents/bubbles_shared/operating-baseline.md (R3).
#
# operating-baseline.md R3 forbids removing a module from an orchestrator's
# closure until a held-out eval shows ZERO gate-detection regression. That is
# only actionable if a regression NAMES the gate it lost, so this suite pins the
# three behaviours the reduction work depends on:
#   both gates reported   -> passes
#   one gate missing      -> FAILS naming the missing gate
#   gatesDetected absent  -> fails closed, never silently passes
#
# Hermetic: the judge is a local fixture, so no model or daemon is required.

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "$0")" && pwd)"
HARNESS="$SCRIPT_DIR/eval-harness.sh"

passed=0
failed=0
pass() { printf 'PASS  %s\n' "$1"; passed=$((passed + 1)); }
fail() { printf 'FAIL  %s\n' "$1"; printf '      %s\n' "${2:-}"; failed=$((failed + 1)); }

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT INT TERM
mkdir -p "$WORK/oracles" "$WORK/out"

printf '%s\n' '#!/usr/bin/env bash' 'set -euo pipefail' 'echo ok' > "$WORK/oracles/ok.sh"
chmod +x "$WORK/oracles/ok.sh"
printf 'artifact\n' > "$WORK/out/artifact.txt"

cat > "$WORK/judge.sh" <<'JUDGE'
#!/usr/bin/env bash
set -euo pipefail
python3 - <<'PY'
import json, os
result = {
    "status": "passed",
    "score": 1.0,
    "verdict": "fixture verdict",
    "rubricFindings": ["fixture finding"],
    "provenance": {"adapter": "gate-detection-fixture", "version": "1.0.0"},
}
if os.environ.get("BUBBLES_FIXTURE_OMIT_GATES") != "1":
    result["gatesDetected"] = [g for g in os.environ.get("BUBBLES_FIXTURE_GATES", "").split(",") if g]
print(json.dumps(result, sort_keys=True))
PY
JUDGE
chmod +x "$WORK/judge.sh"

cat > "$WORK/task.json" <<'TASK'
{
  "schemaVersion": 2,
  "taskId": "gate-detection-selftest",
  "title": "Gate detection selftest",
  "rationale": "Routing scenario that must still raise G019 and G021.",
  "passThreshold": 0.8,
  "judgeWeight": 1,
  "judgeTimeoutSeconds": 60,
  "expectedGates": ["G019", "G021"],
  "checks": [
    {
      "id": "trivial-oracle",
      "type": "executable-oracle",
      "required": true,
      "weight": 1,
      "allowedRoot": "oracles",
      "argv": ["ok.sh"],
      "timeoutSeconds": 10
    }
  ]
}
TASK

echo "Running gate-detection selftest..."
echo "Scenario: a lost gate must fail the eval and name the gate that was lost."

judge_field() {
  BUBBLES_SELFTEST_RAW="$1" BUBBLES_SELFTEST_KEY="$2" python3 - <<'PY'
import json, os
try:
    doc = json.loads(os.environ["BUBBLES_SELFTEST_RAW"])
except ValueError:
    print("<not-json>")
else:
    cur = doc
    for part in os.environ["BUBBLES_SELFTEST_KEY"].split("."):
        cur = cur.get(part) if isinstance(cur, dict) else None
    print(json.dumps(cur) if isinstance(cur, (list, dict)) else ("<missing>" if cur is None else cur))
PY
}

run_harness() {
  BUBBLES_EVAL_JUDGE="$WORK/judge.sh" "$@" \
    bash "$HARNESS" score --task "$WORK/task.json" --output "$WORK/out" 2>/dev/null || true
}

green="$(run_harness env BUBBLES_FIXTURE_GATES=G019,G021)"
[[ "$(judge_field "$green" judge.status)" == "passed" ]] \
  && pass "all expected gates detected -> judge passes" \
  || fail "all expected gates detected -> judge passes" "status=$(judge_field "$green" judge.status)"

red="$(run_harness env BUBBLES_FIXTURE_GATES=G019)"
red_status="$(judge_field "$red" judge.status)"
red_findings="$(judge_field "$red" judge.rubricFindings)"
if [[ "$red_status" == "failed" ]]; then
  pass "a lost gate fails the eval"
else
  fail "a lost gate fails the eval" "status=$red_status (fixture reported only G019)"
fi
if [[ "$red_findings" == *"G021"* ]]; then
  pass "the failure NAMES the lost gate"
else
  fail "the failure NAMES the lost gate" "findings did not mention G021: $red_findings"
fi
# The gate that was still detected must not be reported as lost.
if [[ "$red_findings" == *"G019 was expected but not detected"* ]]; then
  fail "a detected gate is not reported as lost" "G019 was detected but reported missing"
else
  pass "a detected gate is not reported as lost"
fi

omitted="$(run_harness env BUBBLES_FIXTURE_OMIT_GATES=1)"
omitted_status="$(judge_field "$omitted" judge.status)"
if [[ "$omitted_status" == "passed" ]]; then
  fail "an omitted gatesDetected cannot pass" "status=passed — gate detection silently skipped"
else
  pass "an omitted gatesDetected cannot pass (status=$omitted_status)"
fi
[[ "$(judge_field "$omitted" judge.error.code)" == "judge-gates-missing" ]] \
  && pass "omitted gatesDetected reports judge-gates-missing" \
  || fail "omitted gatesDetected reports judge-gates-missing" "code=$(judge_field "$omitted" judge.error.code)"

# expectedGates without a weighted judge is unenforceable and must be rejected.
python3 - "$WORK/task.json" "$WORK/task-noweight.json" <<'PY'
import json, sys
task = json.load(open(sys.argv[1]))
task["judgeWeight"] = 0
task["taskId"] = "gate-detection-selftest-noweight"
json.dump(task, open(sys.argv[2], "w"))
PY
noweight="$(BUBBLES_EVAL_JUDGE="$WORK/judge.sh" bash "$HARNESS" score --task "$WORK/task-noweight.json" --output "$WORK/out" 2>/dev/null || true)"
if [[ "$noweight" == *"expectedGates"* && "$noweight" == *"judgeWeight above zero"* ]]; then
  pass "expectedGates without a weighted judge is rejected"
else
  fail "expectedGates without a weighted judge is rejected" "validator did not flag the dependency"
fi

echo
echo "gate-detection selftest: $passed passed, $failed failed"
[[ "$failed" -eq 0 ]] || exit 1
echo "All gate-detection selftests passed."
