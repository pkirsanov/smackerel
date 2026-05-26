#!/usr/bin/env bash
set -euo pipefail

# Hermetic selftest for Gate G086 — orchestrator_persistence_lint_gate.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/orchestrator-persistence-lint.sh"

if [[ ! -x "$GUARD" ]]; then
  echo "orchestrator-persistence-lint-selftest: guard not executable at $GUARD" >&2
  exit 2
fi

TARGET_FILES=(
  "agents/bubbles.goal.agent.md"
  "agents/bubbles.workflow.agent.md"
  "agents/bubbles.iterate.agent.md"
  "agents/bubbles.sprint.agent.md"
)

WORKSPACE="$(mktemp -d -t bubbles-g086-selftest-XXXXXXXX)"
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
  mkdir -p "$repo/.specify/memory"
  printf '%s' "$repo"
}

write_prompt() {
  local path="$1"
  local extra="$2"
  mkdir -p "$(dirname "$path")"
  cat > "$path" <<EOF
# Fixture prompt

## Orchestrator Persistence Default (Gate G086)

Gate G086 enforces the orchestrator persistence default: after any non-terminal phase, this orchestrator MUST automatically continue to the next phase. It may stop only for convergence achieved, max iterations reached, user requests stop, or fundamental impossibility.

$extra
EOF
}

write_all_clean() {
  local repo="$1"
  local rel
  for rel in "${TARGET_FILES[@]}"; do
    write_prompt "$repo/$rel" "Clean persistence-default fixture for $rel."
  done
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

echo "=== orchestrator-persistence-lint-selftest (Gate G086) ==="

echo ""
echo "--- S0: clean prompt fixtures pass ---"
repo="$(stage_repo s0-clean)"
write_all_clean "$repo"
run_guard "$repo"
assert_exit "S0 clean fixtures" 0
assert_stdout_contains "S0" "PASS Gate G086"
assert_stdout_contains "S0" "scannedFiles=4"

echo ""
echo "--- S1: active forbidden prompt language fails ---"
repo="$(stage_repo s1-forbidden)"
write_all_clean "$repo"
write_prompt "$repo/agents/bubbles.workflow.agent.md" "Active prompt text: shall I proceed after this phase?"
run_guard "$repo"
assert_exit "S1 forbidden phrase" 1
assert_stderr_contains "S1" "G086"
assert_stderr_contains "S1" "shall i proceed"
assert_stderr_contains "S1" "agents/bubbles.workflow.agent.md"

echo ""
echo "--- S2: explicit FORBIDDEN example is exempt ---"
repo="$(stage_repo s2-forbidden-example)"
write_all_clean "$repo"
write_prompt "$repo/agents/bubbles.goal.agent.md" "FORBIDDEN example:\n\`\`\`text\nshall I proceed\n\`\`\`"
run_guard "$repo"
assert_exit "S2 forbidden example" 0
assert_stdout_contains "S2" "PASS Gate G086"

echo ""
echo "--- S3: missing target file exits 2 ---"
repo="$(stage_repo s3-missing)"
write_all_clean "$repo"
rm "$repo/agents/bubbles.sprint.agent.md"
run_guard "$repo"
assert_exit "S3 missing target" 2
assert_stderr_contains "S3" "missing target file"
assert_stderr_contains "S3" "agents/bubbles.sprint.agent.md"

echo ""
echo "=== Selftest verdict ==="
printf '  Total assertions: %d\n' "$((PASS_COUNT + FAIL_COUNT))"
printf '  Passed:           %d\n' "$PASS_COUNT"
printf '  Failed:           %d\n' "$FAIL_COUNT"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
  echo "orchestrator-persistence-lint-selftest: FAILED" >&2
  for scenario in "${FAILED_SCENARIOS[@]}"; do
    echo "  - $scenario" >&2
  done
  exit 1
fi

echo "orchestrator-persistence-lint-selftest: PASSED"
exit 0