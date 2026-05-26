#!/usr/bin/env bash
set -euo pipefail

# Hermetic selftest for Gate G087 - planning_packet_implementation_linkage_gate.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/planning-packet-linkage-guard.sh"

if [[ ! -x "$GUARD" ]]; then
  echo "planning-packet-linkage-guard-selftest: guard not executable at $GUARD" >&2
  exit 2
fi

WORKSPACE="$(mktemp -d -t bubbles-g087-selftest-XXXXXXXX)"
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
  mkdir -p "$repo/specs/100-planning-packet" "$repo/specs/200-implementation"
  printf '%s' "$repo"
}

write_planning_state() {
  local repo="$1"
  local status="$2"
  local planning_only="$3"
  local justification="$4"
  local linked_implementation="$5"
  cat > "$repo/specs/100-planning-packet/state.json" <<EOF
{
  "version": 3,
  "featureDir": "specs/100-planning-packet",
  "featureName": "Planning Packet Fixture",
  "status": "$status",
  "workflowMode": "spec-scope-hardening",
  "linkedImplementationSpec": $linked_implementation,
  "linkedPlanningPacket": null,
  "planningOnly": $planning_only,
  "planningOnlyJustification": $justification,
  "specDependsOn": [],
  "certifiedAt": null,
  "requiresRevalidation": false,
  "executionHistory": []
}
EOF
}

write_implementation_state() {
  local repo="$1"
  local status="$2"
  local linked_planning_packet="$3"
  cat > "$repo/specs/200-implementation/state.json" <<EOF
{
  "version": 3,
  "featureDir": "specs/200-implementation",
  "featureName": "Implementation Fixture",
  "status": "$status",
  "workflowMode": "full-delivery",
  "linkedImplementationSpec": null,
  "linkedPlanningPacket": $linked_planning_packet,
  "planningOnly": false,
  "planningOnlyJustification": null,
  "specDependsOn": [],
  "certifiedAt": null,
  "requiresRevalidation": false,
  "executionHistory": []
}
EOF
}

run_guard() {
  local repo="$1"
  set +e
  bash "$GUARD" "$repo/specs/100-planning-packet" > "$WORKSPACE/stdout.last" 2> "$WORKSPACE/stderr.last"
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

echo "=== planning-packet-linkage-guard-selftest (Gate G087) ==="

echo ""
echo "--- S0: clean linked pair passes ---"
repo="$(stage_repo s0-clean-linked-pair)"
write_planning_state "$repo" "specs_hardened" "false" "null" '"specs/200-implementation"'
write_implementation_state "$repo" "done" '"specs/100-planning-packet"'
run_guard "$repo"
assert_exit "S0 clean linked pair" 0
assert_stdout_contains "S0" "PASS Gate G087"
assert_stdout_contains "S0" "planning_packet_implementation_linkage_gate"

echo ""
echo "--- S1: missing forward linkedImplementationSpec fails ---"
repo="$(stage_repo s1-missing-forward)"
write_planning_state "$repo" "specs_hardened" "false" "null" "null"
write_implementation_state "$repo" "in_progress" "null"
run_guard "$repo"
assert_exit "S1 missing forward link" 1
assert_stderr_contains "S1" "G087"
assert_stderr_contains "S1" "linkedImplementationSpec"

echo ""
echo "--- S2: dangling implementation pointer fails ---"
repo="$(stage_repo s2-dangling-pointer)"
write_planning_state "$repo" "specs_hardened" "false" "null" '"specs/999-missing-implementation"'
rm -rf "$repo/specs/200-implementation"
run_guard "$repo"
assert_exit "S2 dangling pointer" 1
assert_stderr_contains "S2" "G087"
assert_stderr_contains "S2" "dangling"
assert_stderr_contains "S2" "specs/999-missing-implementation"

echo ""
echo "--- S3: done implementation missing reciprocal link fails ---"
repo="$(stage_repo s3-missing-backward)"
write_planning_state "$repo" "specs_hardened" "false" "null" '"specs/200-implementation"'
write_implementation_state "$repo" "done" "null"
run_guard "$repo"
assert_exit "S3 missing back-link" 1
assert_stderr_contains "S3" "G087"
assert_stderr_contains "S3" "linkedPlanningPacket"
assert_stderr_contains "S3" "point back"

echo ""
echo "--- S4: planningOnly true without justification fails ---"
repo="$(stage_repo s4-planning-only-empty)"
write_planning_state "$repo" "specs_hardened" "true" '""' "null"
write_implementation_state "$repo" "in_progress" "null"
run_guard "$repo"
assert_exit "S4 empty justification" 1
assert_stderr_contains "S4" "G087"
assert_stderr_contains "S4" "planningOnlyJustification"

echo ""
echo "--- S5: Smackerel-053-shaped planningOnly opt-out passes ---"
repo="$(stage_repo s5-planning-only-valid)"
write_planning_state "$repo" "specs_hardened" "true" '"CI ops evidence hardening packet with no runtime implementation surface."' "null"
write_implementation_state "$repo" "in_progress" "null"
run_guard "$repo"
assert_exit "S5 planning-only opt-out" 0
assert_stdout_contains "S5" "PASS Gate G087"
assert_stdout_contains "S5" "planningOnly=true"

echo ""
echo "--- S6: archived implementation target fails explicitly ---"
repo="$(stage_repo s6-archived-target)"
write_planning_state "$repo" "specs_hardened" "false" "null" '"specs/200-implementation"'
write_implementation_state "$repo" "archived" '"specs/100-planning-packet"'
run_guard "$repo"
assert_exit "S6 archived target" 1
assert_stderr_contains "S6" "G087"
assert_stderr_contains "S6" "archived implementation target"
assert_stderr_contains "S6" "relink to an active implementation spec"
assert_stderr_contains "S6" "planningOnly:true"

echo ""
echo "=== Selftest verdict ==="
printf '  Total assertions: %d\n' "$((PASS_COUNT + FAIL_COUNT))"
printf '  Passed:           %d\n' "$PASS_COUNT"
printf '  Failed:           %d\n' "$FAIL_COUNT"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
  echo "planning-packet-linkage-guard-selftest: FAILED" >&2
  for scenario in "${FAILED_SCENARIOS[@]}"; do
    echo "  - $scenario" >&2
  done
  exit 1
fi

echo "planning-packet-linkage-guard-selftest: PASSED"
exit 0