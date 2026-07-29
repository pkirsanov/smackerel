#!/usr/bin/env bash
# Judge adapter contract selftest — bubbles/adapters/judge/ollama.sh
#
# HERMETIC part (always runs, no daemon required): proves every failure path is
# FAIL-CLOSED. A judge is required scoring whenever judgeWeight > 0, so the one
# behaviour that must never occur is emitting a passing score without a model.
#
# LIVE part (opportunistic): runs only when BUBBLES_EVAL_JUDGE_URL points at a
# reachable daemon. It asserts the adapter's output is accepted by the harness's
# OWN validator and that a model actually graded — so the hermetic fixtures
# cannot be the only thing this suite ever sees.

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "$0")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/../.." && pwd)"
ADAPTER="$REPO_ROOT/bubbles/adapters/judge/ollama.sh"
HARNESS="$SCRIPT_DIR/eval-harness.sh"

passed=0
failed=0

pass() {
  printf 'PASS  %s\n' "$1"
  passed=$((passed + 1))
}

fail() {
  printf 'FAIL  %s\n' "$1"
  printf '      %s\n' "${2:-}"
  failed=$((failed + 1))
}

TMPDIR_SELF="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_SELF"' EXIT INT TERM

mkdir -p "$TMPDIR_SELF/oracles" "$TMPDIR_SELF/out"
cat > "$TMPDIR_SELF/oracles/ok.sh" <<'ORACLE'
#!/usr/bin/env bash
set -euo pipefail
echo "oracle: ok"
ORACLE
chmod +x "$TMPDIR_SELF/oracles/ok.sh"

cat > "$TMPDIR_SELF/out/report.md" <<'REPORT'
# Report
## Summary
Implemented the feature. Everything works.
## Test Evidence
All tests pass. Exit Code: 0.
## Completion Statement
Done.
REPORT

cat > "$TMPDIR_SELF/task.json" <<'TASK'
{
  "schemaVersion": 2,
  "taskId": "judge-adapter-contract-selftest",
  "title": "Judge adapter contract selftest",
  "rationale": "The agent claimed every test passed but produced no terminal output, no exit code, and no command. Grade whether the claim is supported by evidence.",
  "passThreshold": 0.8,
  "judgeWeight": 1,
  "judgeTimeoutSeconds": 300,
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

echo "Running judge-adapter contract selftest..."
echo "Scenario: a judge adapter must fail closed on every error path and never invent a passing score."

[[ -f "$ADAPTER" ]] || { fail "adapter exists" "$ADAPTER not found"; echo "judge-adapter contract selftest: $passed passed, $failed failed"; exit 1; }
[[ -x "$ADAPTER" ]] && pass "adapter is executable" || fail "adapter is executable" "$ADAPTER is not executable"

# Reads one field out of an adapter's raw stdout.
adapter_field() {
  BUBBLES_SELFTEST_RAW="$1" BUBBLES_SELFTEST_KEY="$2" python3 - <<'PY'
import json, os
try:
    doc = json.loads(os.environ["BUBBLES_SELFTEST_RAW"])
except ValueError:
    print("<not-json>")
else:
    key = os.environ["BUBBLES_SELFTEST_KEY"]
    cur = doc
    for part in key.split("."):
        cur = cur.get(part) if isinstance(cur, dict) else None
    print("<missing>" if cur is None else cur)
PY
}

expect_adapter() {
  local label="$1" expect_status="$2" expect_code="$3"
  shift 3
  local raw status code score
  raw="$("$@" 2>/dev/null || true)"
  status="$(adapter_field "$raw" status)"
  code="$(adapter_field "$raw" error.code)"
  score="$(adapter_field "$raw" score)"
  if [[ "$status" == "$expect_status" && "$code" == "$expect_code" && "$score" == "<missing>" ]]; then
    pass "$label"
  else
    fail "$label" "status=$status code=$code score=$score (wanted status=$expect_status code=$expect_code score=null)"
  fi
}

BUBBLES_EVAL_JUDGE_URL="" expect_adapter \
  "unset endpoint fails closed with a null score" unavailable judge-url-unset \
  bash "$ADAPTER" "$TMPDIR_SELF/out" "$TMPDIR_SELF/task.json"

BUBBLES_EVAL_JUDGE_URL="http://127.0.0.1:1" expect_adapter \
  "unreachable endpoint fails closed with a null score" unavailable judge-endpoint-unreachable \
  bash "$ADAPTER" "$TMPDIR_SELF/out" "$TMPDIR_SELF/task.json"

BUBBLES_EVAL_JUDGE_URL="http://127.0.0.1:1" expect_adapter \
  "missing out_dir fails closed" error judge-out-dir-missing \
  bash "$ADAPTER" "$TMPDIR_SELF/does-not-exist" "$TMPDIR_SELF/task.json"

BUBBLES_EVAL_JUDGE_URL="http://127.0.0.1:1" expect_adapter \
  "unreadable task fails closed" error judge-task-unreadable \
  bash "$ADAPTER" "$TMPDIR_SELF/out" "$TMPDIR_SELF/no-such-task.json"

BUBBLES_EVAL_JUDGE_URL="http://127.0.0.1:1" expect_adapter \
  "missing arguments fail closed" error judge-usage \
  bash "$ADAPTER"

# ADVERSARIAL: the failure that would matter. No endpoint can ever yield a pass.
adversarial_raw="$(BUBBLES_EVAL_JUDGE_URL="" bash "$ADAPTER" "$TMPDIR_SELF/out" "$TMPDIR_SELF/task.json" 2>/dev/null || true)"
adversarial_status="$(adapter_field "$adversarial_raw" status)"
if [[ "$adversarial_status" == "passed" ]]; then
  fail "no endpoint can never produce a passing verdict" "adapter emitted status=passed with no judge endpoint"
else
  pass "no endpoint can never produce a passing verdict"
fi

# The harness's OWN validator must accept the shape; provenance is mandatory.
harness_out="$(BUBBLES_EVAL_JUDGE="$ADAPTER" BUBBLES_EVAL_JUDGE_URL="http://127.0.0.1:1" \
  bash "$HARNESS" score --task "$TMPDIR_SELF/task.json" --output "$TMPDIR_SELF/out" 2>/dev/null || true)"
harness_adapter="$(adapter_field "$harness_out" judge.provenance.adapter)"
harness_code="$(adapter_field "$harness_out" judge.error.code)"
if [[ "$harness_adapter" == "ollama" && "$harness_code" == "judge-endpoint-unreachable" ]]; then
  pass "harness accepts the adapter result shape and preserves provenance"
else
  fail "harness accepts the adapter result shape and preserves provenance" \
    "provenance.adapter=$harness_adapter judge.error.code=$harness_code"
fi

# LIVE (opportunistic): only when an operator has pointed at a real daemon.
if [[ -n "${BUBBLES_EVAL_JUDGE_URL:-}" ]] && curl -s --max-time 5 "${BUBBLES_EVAL_JUDGE_URL%/}/api/tags" >/dev/null 2>&1; then
  live_out="$(BUBBLES_EVAL_JUDGE="$ADAPTER" bash "$HARNESS" score \
    --task "$TMPDIR_SELF/task.json" --output "$TMPDIR_SELF/out" 2>/dev/null || true)"
  live_status="$(adapter_field "$live_out" judge.status)"
  live_model="$(adapter_field "$live_out" judge.provenance.model)"
  if [[ "$live_status" == "passed" || "$live_status" == "failed" ]]; then
    pass "live judge returns a real verdict (status=$live_status model=$live_model)"
  else
    fail "live judge returns a real verdict" "judge.status=$live_status"
  fi
  # The fixture claims success with zero evidence, so a working judge must reject it.
  if [[ "$live_status" == "failed" ]]; then
    pass "live judge rejects an evidence-free report"
  else
    fail "live judge rejects an evidence-free report" "judge.status=$live_status (expected failed)"
  fi
else
  echo "SKIP  live judge checks (set BUBBLES_EVAL_JUDGE_URL to a reachable daemon to enable)"
fi

echo
echo "judge-adapter contract selftest: $passed passed, $failed failed"
[[ "$failed" -eq 0 ]] || exit 1
echo "All judge-adapter contract selftests passed."
