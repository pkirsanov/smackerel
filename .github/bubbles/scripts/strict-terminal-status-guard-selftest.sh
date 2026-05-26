#!/usr/bin/env bash
set -euo pipefail

# Hermetic selftest for Gate G092 - strict_terminal_status_gate.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/strict-terminal-status-guard.sh"
DEPENDENCY_GUARD="$SCRIPT_DIR/inter-spec-dependency-guard.sh"

if [[ ! -x "$GUARD" ]]; then
  echo "strict-terminal-status-guard-selftest: guard not executable at $GUARD" >&2
  exit 2
fi

if [[ ! -x "$DEPENDENCY_GUARD" ]]; then
  echo "strict-terminal-status-guard-selftest: dependency guard not executable at $DEPENDENCY_GUARD" >&2
  exit 2
fi

WORKSPACE="$(mktemp -d -t bubbles-g092-selftest-XXXXXXXX)"
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
  mkdir -p "$repo/specs/100-current" "$repo/bubbles" "$repo/agents/bubbles_shared" "$repo/agents" "$repo/bubbles/scripts" "$repo/tests/regression"
  write_clean_workflows "$repo"
  write_clean_text_contracts "$repo"
  printf '%s' "$repo"
}

write_clean_workflows() {
  local repo="$1"
  cat > "$repo/bubbles/workflows.yaml" <<'EOF'
version: 1
gates:
  G092:
    name: strict_terminal_status_gate
    description: Legacy done_with_concerns is read-only compatibility only; new writes are forbidden.
outcomeStates:
  done:
    description: Done with optional non-status observations.
    requiresEvidence: true
    supportsObservations: true
    permittedFollowUps: []
  blocked:
    description: Required work remains.
    requiresEvidence: true
    permittedFollowUps: ["bugfix-fastlane"]
legacyOutcomeStates:
  done_with_concerns:
    description: Legacy read-only compatibility state; migration writes done plus observations or blocked.
    readOnlyCompatibility: true
EOF
}

write_clean_text_contracts() {
  local repo="$1"
  cat > "$repo/agents/bubbles_shared/completion-governance.md" <<'EOF'
# Completion Governance

New terminal certification writes may use `done` or `blocked` only. Legacy `done_with_concerns` is read-only compatibility metadata until migration, and migrated notes live in observations[].
EOF
  for agent in bubbles.validate.agent.md bubbles.workflow.agent.md bubbles.audit.agent.md bubbles.harden.agent.md bubbles.gaps.agent.md bubbles.retro.agent.md bubbles.spec-review.agent.md; do
    cat > "$repo/agents/$agent" <<'EOF'
# Agent Fixture

Use observations[] for non-blocking notes. Legacy done_with_concerns is read-only compatibility only and MUST NOT be emitted as a new outcome.
EOF
  done
  cat > "$repo/bubbles/scripts/post-cert-spec-edit-guard.sh" <<'EOF'
# G088 fixture: legacy done_with_concerns is read-only compatible until touched; recertification migrates to done plus observations or blocked.
EOF
  cat > "$repo/bubbles/scripts/post-cert-spec-edit-guard-selftest.sh" <<'EOF'
# G088 selftest fixture: legacy read-only done_with_concerns compatibility.
EOF
  cat > "$repo/bubbles/scripts/inter-spec-dependency-guard.sh" <<'EOF'
# G089 fixture: legacy done_with_concerns dependencies are read-only compatibility only; new done_with_concerns is forbidden.
EOF
  cat > "$repo/bubbles/scripts/inter-spec-dependency-guard-selftest.sh" <<'EOF'
# G089 selftest fixture: legacy read-only done_with_concerns compatibility.
EOF
  cat > "$repo/tests/regression/test_11_post_cert_spec_edit.sh" <<'EOF'
# G088 regression fixture: legacy read-only done_with_concerns compatibility.
EOF
  cat > "$repo/tests/regression/test_12_inter_spec_dependency.sh" <<'EOF'
# G089 regression fixture: legacy read-only done_with_concerns compatibility.
EOF
}

write_state() {
  local repo="$1"
  local spec_rel="$2"
  local status="$3"
  local cert_status="$4"
  local observations_json="$5"
  local extra_fields="${6:-}"
  mkdir -p "$repo/$spec_rel"
  cat > "$repo/$spec_rel/state.json" <<EOF
{
  "version": 3,
  "featureDir": "$spec_rel",
  "featureName": "G092 Fixture",
  "status": "$status",
  "workflowMode": "full-delivery",
  "observations": $observations_json,
  "certification": {
    "status": "$cert_status",
    "observations": $observations_json
  },
  "specDependsOn": [],
  "requiresRevalidation": false,
  "executionHistory": []$extra_fields
}
EOF
}

run_guard() {
  local repo="$1"
  local spec_rel="$2"
  set +e
  bash "$GUARD" "$repo/$spec_rel" --repo-root "$repo" > "$WORKSPACE/stdout.last" 2> "$WORKSPACE/stderr.last"
  local rc=$?
  set -e
  echo "$rc" > "$WORKSPACE/exit.last"
}

run_dependency_guard() {
  local repo="$1"
  local spec_rel="$2"
  set +e
  bash "$DEPENDENCY_GUARD" "$repo/$spec_rel" --repo-root "$repo" > "$WORKSPACE/stdout.last" 2> "$WORKSPACE/stderr.last"
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

echo "=== strict-terminal-status-guard-selftest (Gate G092) ==="

echo ""
echo "--- S1: new certification write to done_with_concerns fails ---"
repo="$(stage_repo s1-new-write)"
write_state "$repo" "specs/100-current" "in_progress" "in_progress" '[]' ',"certificationProvenance":"fixture","proposedStatus":"done_with_concerns"'
run_guard "$repo" "specs/100-current"
assert_exit "S1 new done_with_concerns write" 1
assert_stderr_contains "S1" "G092"
assert_stderr_contains "S1" "done_with_concerns"
assert_stderr_contains "S1" "valid new terminal statuses are done and blocked"

echo ""
echo "--- S2: done with non-blocking observations passes ---"
repo="$(stage_repo s2-done-observations)"
write_state "$repo" "specs/100-current" "done" "done" '[{"severity":"low","summary":"Monitor latency trend."}]'
run_guard "$repo" "specs/100-current"
assert_exit "S2 done observations" 0
assert_stdout_contains "S2" "PASS Gate G092"
assert_stdout_contains "S2" "terminalStatuses=done,blocked"

echo ""
echo "--- S3: legacy done_with_concerns is read-only compatible ---"
repo="$(stage_repo s3-legacy-read-only)"
write_state "$repo" "specs/100-current" "done_with_concerns" "done_with_concerns" '[{"severity":"low","summary":"Legacy migrated concern."}]' ',"legacyStatusCompatibility":true,"certifiedAt":"2026-05-01T00:00:00Z"'
run_guard "$repo" "specs/100-current"
assert_exit "S3 legacy read-only" 0
assert_stdout_contains "S3" "legacyReadOnlyStatus=present"

echo ""
echo "--- S4: touched legacy status must migrate ---"
repo="$(stage_repo s4-touched-legacy)"
write_state "$repo" "specs/100-current" "done_with_concerns" "done_with_concerns" '[{"severity":"low","summary":"Legacy concern."}]' ',"legacyStatusCompatibility":true,"certifiedAt":"2026-05-01T00:00:00Z","touchedForRecertification":true'
run_guard "$repo" "specs/100-current"
assert_exit "S4 touched legacy blocks" 1
assert_stderr_contains "S4 touched" "migrate to status done with observations[]"
repo="$(stage_repo s4-migrated-done)"
write_state "$repo" "specs/100-current" "done" "done" '[{"severity":"low","summary":"Migrated legacy concern."}]'
run_guard "$repo" "specs/100-current"
assert_exit "S4 migrated done passes" 0
repo="$(stage_repo s4-migrated-blocked)"
write_state "$repo" "specs/100-current" "blocked" "blocked" '[{"severity":"high","summary":"Required work remains."}]'
run_guard "$repo" "specs/100-current"
assert_exit "S4 migrated blocked passes" 0

echo ""
echo "--- S5: high or remediation-required observation blocks done ---"
repo="$(stage_repo s5-high-observation)"
write_state "$repo" "specs/100-current" "done" "done" '[{"severity":"high","summary":"Gate risk."}]'
run_guard "$repo" "specs/100-current"
assert_exit "S5 high observation" 1
assert_stderr_contains "S5 high" "high/remediation-required observation"
repo="$(stage_repo s5-remediation-observation)"
write_state "$repo" "specs/100-current" "done" "done" '[{"severity":"medium","summary":"Needs fix.","remediationRequired":true}]'
run_guard "$repo" "specs/100-current"
assert_exit "S5 remediation observation" 1
assert_stderr_contains "S5 remediation" "high/remediation-required observation"

echo ""
echo "--- S6: new done_with_concerns dependency is not stable ---"
repo="$(stage_repo s6-new-dependency)"
write_state "$repo" "specs/100-dependent" "in_progress" "in_progress" '[]'
jq '.specDependsOn = ["specs/200-upstream"]' "$repo/specs/100-dependent/state.json" > "$WORKSPACE/state.tmp"
mv "$WORKSPACE/state.tmp" "$repo/specs/100-dependent/state.json"
write_state "$repo" "specs/200-upstream" "done_with_concerns" "done_with_concerns" '[]' ',"certifiedAt":"2026-05-01T00:00:00Z","certification":{"status":"done_with_concerns","observations":[],"proposedStatus":"done_with_concerns"}'
run_dependency_guard "$repo" "specs/100-dependent"
assert_exit "S6 dependency guard rejects new done_with_concerns" 1
assert_stderr_contains "S6" "G089"
assert_stderr_contains "S6" "G092"
assert_stderr_contains "S6" "legacy read-only"

echo ""
echo "--- S7: active framework text cannot permit new done_with_concerns ---"
repo="$(stage_repo s7-active-text)"
cat > "$repo/agents/bubbles.validate.agent.md" <<'EOF'
# Validate Fixture

Validate MAY emit outcome: done_with_concerns when gates pass.
EOF
write_state "$repo" "specs/100-current" "done" "done" '[]'
run_guard "$repo" "specs/100-current"
assert_exit "S7 active text" 1
assert_stderr_contains "S7" "bubbles.validate.agent.md"
assert_stderr_contains "S7" "active done_with_concerns"

echo ""
echo "=== Selftest verdict ==="
printf '  Total assertions: %d\n' "$((PASS_COUNT + FAIL_COUNT))"
printf '  Passed:           %d\n' "$PASS_COUNT"
printf '  Failed:           %d\n' "$FAIL_COUNT"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
  echo "strict-terminal-status-guard-selftest: FAILED" >&2
  for scenario in "${FAILED_SCENARIOS[@]}"; do
    echo "  - $scenario" >&2
  done
  exit 1
fi

echo "strict-terminal-status-guard-selftest: PASSED"
exit 0