#!/usr/bin/env bash
set -euo pipefail

# Hermetic selftest for Gate G086 — orchestrator_persistence_lint_gate.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/orchestrator-persistence-lint.sh"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/guard-lib.sh"

if [[ ! -x "$GUARD" ]]; then
  echo "orchestrator-persistence-lint-selftest: guard not executable at $GUARD" >&2
  exit 2
fi

TARGET_FILES=(
  "agents/bubbles.goal.agent.md"
  "agents/bubbles.workflow.agent.md"
  "agents/bubbles.iterate.agent.md"
  "agents/bubbles.sprint.agent.md"
  "agents/bubbles.bug.agent.md"
  "agents/bubbles.releases.agent.md"
  "agents/bubbles.train.agent.md"
  "agents/bubbles.upkeep.agent.md"
  "agents/bubbles.propagate.agent.md"
  "agents/bubbles.stabilize.agent.md"
  "agents/bubbles.retro.agent.md"
  "agents/bubbles.journey.agent.md"
  "agents/bubbles.super.agent.md"
  "agents/bubbles.code-review.agent.md"
  "agents/bubbles.system-review.agent.md"
)

WORKSPACE="$(mktemp -d -t bubbles-g086-selftest-XXXXXXXX)"
# shellcheck disable=SC2329 # Invoked indirectly by trap.
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

stage_downstream_repo() {
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

Classify raw input first with bubbles/scripts/continuation-intent-resolve.sh.

phase_1_understand:
phase_1_parse_and_estimate:
### Phase 0: Resolve Inputs
## Scope Selection Priority

## Terminal Recap Boundary

At a terminal stop, invoke runSubagent(bubbles.recap) and return control to the user.

$extra
EOF
}

write_all_clean() {
  local repo="$1"
  local prefix="${2:-}"
  local rel
  mkdir -p "$repo/${prefix}bubbles"
  mkdir -p "$repo/${prefix}bubbles/scripts" "$repo/$prefix/agents/bubbles_shared"
  {
    echo "workflowModeGrants:"
    echo "  agents:"
    for rel in "${TARGET_FILES[@]}"; do
      agent_name="$(basename "$rel" .agent.md)"
      echo "    ${agent_name}:"
      echo "      modes: [ test-mode ]"
    done
    echo "resultPolicy:"
    echo "  allowedOutcomes: [ completed_owned ]"
    echo "terminalRecapPolicy:"
    echo "  directInvocationOnly: true"
    echo "  phaseOwnersReturnUpward: true"
    echo "  additionalAgents:"
    echo "  - bubbles.super"
    echo "  - bubbles.code-review"
    echo "  - bubbles.system-review"
  } > "$repo/${prefix}bubbles/agent-capabilities.yaml"
  cat > "$repo/${prefix}bubbles/scripts/continuation-intent-resolve.sh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' CONTINUE
EOF
  chmod +x "$repo/${prefix}bubbles/scripts/continuation-intent-resolve.sh"
  cat > "$repo/$prefix/agents/bubbles_shared/agent-common.md" <<'EOF'
A dispatched phase-owner subagent MUST NOT invoke recap. It returns upward.
EOF
  cat > "$repo/$prefix/agents/bubbles.recap.agent.md" <<'EOF'
# Recap fixture

`bubbles.recap` never invokes itself.
EOF
  for rel in "${TARGET_FILES[@]}"; do
    write_prompt "$repo/$prefix$rel" "Clean persistence-default fixture for $rel."
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
assert_stdout_contains "S0" "persistenceFiles=4"
assert_stdout_contains "S0" "recapFiles=15"

echo ""
echo "--- S0b: downstream .github-installed prompt fixtures pass ---"
repo="$(stage_downstream_repo s0b-downstream-layout)"
write_all_clean "$repo" ".github/"
run_guard "$repo"
assert_exit "S0b downstream fixtures" 0
assert_stdout_contains "S0b" "PASS Gate G086"
assert_stdout_contains "S0b" "persistenceFiles=4"
assert_stdout_contains "S0b" "recapFiles=15"

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
echo "--- S4: missing terminal recap contract fails ---"
repo="$(stage_repo s4-missing-terminal-recap)"
write_all_clean "$repo"
cat > "$repo/agents/bubbles.goal.agent.md" <<'EOF'
# Fixture prompt

## Orchestrator Persistence Default (Gate G086)

Gate G086 enforces the orchestrator persistence default: after any non-terminal phase, this orchestrator MUST automatically continue to the next phase. It may stop only for convergence achieved, max iterations reached, user requests stop, or fundamental impossibility.
EOF
run_guard "$repo"
assert_exit "S4 missing terminal recap" 1
assert_stderr_contains "S4" "terminal recap boundary"
assert_stderr_contains "S4" "runSubagent(bubbles.recap)"

echo ""
echo "--- S5: a newly granted runner without recap fails automatically ---"
repo="$(stage_repo s5-new-granted-runner)"
write_all_clean "$repo"
cat > "$repo/bubbles/agent-capabilities.yaml" <<'EOF'
workflowModeGrants:
  agents:
    bubbles.workflow:
      modes: [ test-mode ]
    bubbles.goal:
      modes: [ test-mode ]
    bubbles.sprint:
      modes: [ test-mode ]
    bubbles.iterate:
      modes: [ test-mode ]
    bubbles.bug:
      modes: [ test-mode ]
    bubbles.releases:
      modes: [ test-mode ]
    bubbles.train:
      modes: [ test-mode ]
    bubbles.upkeep:
      modes: [ test-mode ]
    bubbles.propagate:
      modes: [ test-mode ]
    bubbles.stabilize:
      modes: [ test-mode ]
    bubbles.retro:
      modes: [ test-mode ]
    bubbles.journey:
      modes: [ test-mode ]
    bubbles.new-runner:
      modes: [ test-mode ]
resultPolicy:
  allowedOutcomes: [ completed_owned ]
EOF
mkdir -p "$repo/agents"
cat > "$repo/agents/bubbles.new-runner.agent.md" <<'EOF'
# Fixture prompt

This newly granted runner intentionally omits the completion-summary contract.
EOF
run_guard "$repo"
assert_exit "S5 new granted runner" 1
assert_stderr_contains "S5" "agents/bubbles.new-runner.agent.md"
assert_stderr_contains "S5" "runSubagent(bubbles.recap)"

echo ""
echo "--- S6: direct top-level super without recap fails ---"
repo="$(stage_repo s6-super-missing-recap)"
write_all_clean "$repo"
cat > "$repo/agents/bubbles.super.agent.md" <<'EOF'
# Fixture prompt

This direct framework utility intentionally omits terminal recap.
EOF
run_guard "$repo"
assert_exit "S6 super missing recap" 1
assert_stderr_contains "S6" "agents/bubbles.super.agent.md"
assert_stderr_contains "S6" "runSubagent(bubbles.recap)"

echo ""
echo "--- S7: missing classifier consumption fails ---"
repo="$(stage_repo s7-missing-classifier)"
write_all_clean "$repo"
cat > "$repo/agents/bubbles.goal.agent.md" <<'EOF'
# Fixture prompt

## Orchestrator Persistence Default (Gate G086)

After any non-terminal phase, automatically continue until convergence achieved, max iterations, user requests stop, or fundamental impossibility.

## Terminal Recap Boundary

At a terminal stop, invoke runSubagent(bubbles.recap).
EOF
run_guard "$repo"
assert_exit "S7 missing classifier" 1
assert_stderr_contains "S7" "continuation-intent-resolve.sh"

echo ""
echo "--- S8: phase owners configured to recap fails ---"
repo="$(stage_repo s8-phase-owner-policy)"
write_all_clean "$repo"
bubbles_sed_inplace 's/phaseOwnersReturnUpward: true/phaseOwnersReturnUpward: false/' "$repo/bubbles/agent-capabilities.yaml"
run_guard "$repo"
assert_exit "S8 phase-owner policy" 1
assert_stderr_contains "S8" "phaseOwnersReturnUpward must be true"

echo ""
echo "--- S9: recap self-dispatch fails ---"
repo="$(stage_repo s9-recap-recursion)"
write_all_clean "$repo"
printf '\nRecursive call: runSubagent(bubbles.recap)\n' >> "$repo/agents/bubbles.recap.agent.md"
run_guard "$repo"
assert_exit "S9 recap recursion" 1
assert_stderr_contains "S9" "must not recursively dispatch itself"

echo ""
echo "--- S10: removing a required runner from registry fails ---"
repo="$(stage_repo s10-roster-shrink)"
write_all_clean "$repo"
bubbles_sed_inplace '/^    bubbles.journey:$/,/^      modes:/d' "$repo/bubbles/agent-capabilities.yaml"
run_guard "$repo"
assert_exit "S10 roster shrink" 1
assert_stderr_contains "S10" "required recap agent absent"

echo ""
echo "--- S11: classifier after parser fails ---"
repo="$(stage_repo s11-classifier-order)"
write_all_clean "$repo"
cat > "$repo/agents/bubbles.goal.agent.md" <<'EOF'
# Fixture prompt

## Orchestrator Persistence Default (Gate G086)

After any non-terminal phase, automatically continue until convergence achieved, max iterations, user requests stop, or fundamental impossibility.

phase_1_understand:
Classify late with bubbles/scripts/continuation-intent-resolve.sh.

## Terminal Recap Boundary

At a terminal stop, invoke runSubagent(bubbles.recap).
EOF
run_guard "$repo"
assert_exit "S11 classifier order" 1
assert_stderr_contains "S11" "must classify continuation before 'phase_1_understand:'"

echo ""
echo "--- S12: targeted continue mapped to implement fails ---"
repo="$(stage_repo s12-targeted-implement)"
write_all_clean "$repo"
printf '\n| continue working on booking | feature: booking, type: implement |\n' >> "$repo/agents/bubbles.iterate.agent.md"
run_guard "$repo"
assert_exit "S12 targeted implement" 1
assert_stderr_contains "S12" "maps continuation to new implementation work"

echo ""
echo "--- S13: extra registry recap agent fails independently ---"
repo="$(stage_repo s13-roster-growth)"
write_all_clean "$repo"
bubbles_sed_inplace '/^  - bubbles.system-review$/a\
  - bubbles.new-runner' "$repo/bubbles/agent-capabilities.yaml"
write_prompt "$repo/agents/bubbles.new-runner.agent.md" "Valid recap-bearing extra runner."
run_guard "$repo"
assert_exit "S13 roster growth" 1
assert_stderr_contains "S13" "unreviewed recap agent added"

echo ""
echo "--- S14-S16: workflow, iterate, and sprint late classifiers fail ---"
for runner_case in \
  "workflow|agents/bubbles.workflow.agent.md|### Phase 0: Resolve Inputs" \
  "iterate|agents/bubbles.iterate.agent.md|## Scope Selection Priority" \
  "sprint|agents/bubbles.sprint.agent.md|phase_1_parse_and_estimate:"; do
  IFS='|' read -r runner_name runner_path parser_marker <<<"$runner_case"
  repo="$(stage_repo "s14-${runner_name}-late-classifier")"
  write_all_clean "$repo"
  cat > "$repo/$runner_path" <<EOF
# Fixture prompt

## Orchestrator Persistence Default (Gate G086)

After any non-terminal phase, automatically continue until convergence achieved, max iterations, user requests stop, or fundamental impossibility.

$parser_marker
Classify late with bubbles/scripts/continuation-intent-resolve.sh.

## Terminal Recap Boundary

At a terminal stop, invoke runSubagent(bubbles.recap).
EOF
  run_guard "$repo"
  assert_exit "late classifier $runner_name" 1
  assert_stderr_contains "late classifier $runner_name" "must classify continuation before '$parser_marker'"
done

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