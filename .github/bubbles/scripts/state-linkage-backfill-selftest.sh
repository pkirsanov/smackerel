#!/usr/bin/env bash
set -euo pipefail

# Hermetic selftest for SCOPE-9 state linkage schema backfill.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
BACKFILL="$SCRIPT_DIR/state-linkage-backfill.sh"
TEMPLATE="$REPO_ROOT/agents/bubbles_shared/feature-templates.md"

if [[ ! -x "$BACKFILL" && ! -f "$BACKFILL" ]]; then
  echo "state-linkage-backfill-selftest: backfill script missing at $BACKFILL" >&2
  exit 2
fi

WORKSPACE="$(mktemp -d -t bubbles-scope9-selftest-XXXXXXXX)"
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
  mkdir -p "$repo/specs/001-fixture"
  printf '%s' "$repo"
}

run_backfill() {
  set +e
  bash "$BACKFILL" "$@" > "$WORKSPACE/stdout.last" 2> "$WORKSPACE/stderr.last"
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

assert_file_contains() {
  local label="$1"
  local file="$2"
  local needle="$3"
  if grep -qF "$needle" "$file"; then
    ok "$label contains '$needle'"
  else
    ko "$label missing '$needle'"
    cat "$file"
  fi
}

assert_stdout_json_value() {
  local label="$1"
  local jq_expr="$2"
  local expected="$3"
  local actual
  actual="$(jq -r "$jq_expr" "$WORKSPACE/stdout.last")"
  if [[ "$actual" == "$expected" ]]; then
    ok "$label = $actual"
  else
    ko "$label expected '$expected' actual '$actual'"
    cat "$WORKSPACE/stdout.last"
  fi
}

assert_state_value() {
  local label="$1"
  local file="$2"
  local jq_expr="$3"
  local expected="$4"
  local actual
  actual="$(jq -r "$jq_expr" "$file")"
  if [[ "$actual" == "$expected" ]]; then
    ok "$label = $actual"
  else
    ko "$label expected '$expected' actual '$actual'"
    cat "$file"
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

assert_files_same() {
  local label="$1"
  local left="$2"
  local right="$3"
  if cmp -s "$left" "$right"; then
    ok "$label unchanged"
  else
    ko "$label changed unexpectedly"
    diff -u "$left" "$right" || true
  fi
}

write_minimal_state() {
  local file="$1"
  cat > "$file" <<'EOF'
{
  "version": 3,
  "featureDir": "specs/001-fixture",
  "featureName": "Fixture",
  "status": "in_progress",
  "workflowMode": "full-delivery",
  "execution": {
    "activeAgent": "bubbles.implement",
    "currentPhase": "implement",
    "currentScope": null,
    "runStartedAt": "2026-05-24T00:00:00Z",
    "completedPhaseClaims": [],
    "pendingTransitionRequests": []
  },
  "certification": {
    "status": "in_progress",
    "completedScopes": [],
    "certifiedCompletedPhases": [],
    "scopeProgress": [],
    "lockdownState": {
      "active": false,
      "lockedScenarioIds": []
    }
  },
  "policySnapshot": {},
  "transitionRequests": [],
  "reworkQueue": [],
  "executionHistory": []
}
EOF
}

echo "=== state-linkage-backfill-selftest (SCOPE-9) ==="

echo ""
echo "--- S1: state template includes linkage fields ---"
assert_file_contains "S1 template" "$TEMPLATE" '"linkedImplementationSpec": null'
assert_file_contains "S1 template" "$TEMPLATE" '"linkedPlanningPacket": null'
assert_file_contains "S1 template" "$TEMPLATE" '"planningOnly": false'
assert_file_contains "S1 template" "$TEMPLATE" '"planningOnlyJustification": null'
assert_file_contains "S1 template" "$TEMPLATE" '"specDependsOn": []'
assert_file_contains "S1 template" "$TEMPLATE" '"certifiedAt": null'
assert_file_contains "S1 template" "$TEMPLATE" '"requiresRevalidation": false'

echo ""
echo "--- S2: backfill hoists latest certifiedAt and preserves scopeProgress ---"
repo="$(stage_repo s2-hoist)"
state="$repo/specs/001-fixture/state.json"
cat > "$state" <<'EOF'
{
  "version": 3,
  "featureDir": "specs/001-fixture",
  "featureName": "Fixture",
  "status": "in_progress",
  "workflowMode": "full-delivery",
  "certification": {
    "status": "in_progress",
    "scopeProgress": [
      { "scope": 1, "name": "one", "status": "Done", "certifiedAt": "2026-05-24T01:00:00Z" },
      { "scope": 2, "name": "two", "status": "Done", "certifiedCompletedAt": "2026-05-24T03:00:00Z" },
      { "scope": 3, "name": "three", "status": "Done", "certifiedAt": "2026-05-24T02:00:00Z" }
    ]
  }
}
EOF
before_scope="$WORKSPACE/s2-scope-before.json"
jq -c '.certification.scopeProgress' "$state" > "$before_scope"
run_backfill "$state" --apply
assert_exit "S2 apply" 0
assert_state_value "S2 certifiedAt" "$state" '.certifiedAt' "2026-05-24T03:00:00Z"
jq -c '.certification.scopeProgress' "$state" > "$WORKSPACE/s2-scope-after.json"
assert_files_same "S2 scopeProgress" "$before_scope" "$WORKSPACE/s2-scope-after.json"

echo ""
echo "--- S3: already-backfilled state is idempotent ---"
repo="$(stage_repo s3-idempotent)"
state="$repo/specs/001-fixture/state.json"
write_minimal_state "$state"
run_backfill "$state" --apply
assert_exit "S3 first apply" 0
snapshot="$WORKSPACE/s3-after-first.json"
cp "$state" "$snapshot"
run_backfill "$state" --apply
assert_exit "S3 second apply" 0
assert_stderr_contains "S3" "already backfilled"
assert_files_same "S3 state" "$snapshot" "$state"

echo ""
echo "--- S4: Smackerel 053-shaped planning packet is classifiable ---"
repo="$(stage_repo s4-smackerel-053)"
state="$repo/specs/053-ci-ops-evidence-hardening/state.json"
mkdir -p "$(dirname "$state")"
cat > "$state" <<'EOF'
{
  "version": 3,
  "featureDir": "specs/053-ci-ops-evidence-hardening",
  "featureName": "CI Ops Evidence Hardening",
  "status": "specs_hardened",
  "workflowMode": "spec-scope-hardening",
  "certification": {
    "status": "specs_hardened",
    "scopeProgress": [
      { "scope": 1, "name": "audit packet", "status": "Done", "certifiedAt": "2026-05-23T21:30:00Z" },
      { "scope": 2, "name": "ops packet", "status": "Done", "certifiedAt": "2026-05-24T04:15:00Z" }
    ]
  }
}
EOF
run_backfill "$state" --planning-only --planning-only-justification "CI ops evidence hardening packet with no runtime implementation surface." --apply
assert_exit "S4 planning-only apply" 0
assert_state_value "S4 planningOnly" "$state" '.planningOnly' "true"
assert_state_value "S4 justification non-empty" "$state" '(.planningOnlyJustification | length > 0)' "true"
assert_state_value "S4 certifiedAt" "$state" '.certifiedAt' "2026-05-24T04:15:00Z"

echo ""
echo "--- S5: planning-only classification without justification fails loud ---"
repo="$(stage_repo s5-invalid-planning-only)"
state="$repo/specs/001-fixture/state.json"
write_minimal_state "$state"
run_backfill "$state" --planning-only --apply
assert_exit "S5 missing justification" 2
assert_stderr_contains "S5" "requires non-empty"

echo ""
echo "--- S6: dry-run prints explicit additive defaults without modifying file ---"
repo="$(stage_repo s6-dry-run)"
state="$repo/specs/001-fixture/state.json"
write_minimal_state "$state"
cp "$state" "$WORKSPACE/s6-before.json"
run_backfill "$repo/specs/001-fixture"
assert_exit "S6 dry-run" 0
assert_stdout_json_value "S6 linkedImplementationSpec" '.linkedImplementationSpec' "null"
assert_stdout_json_value "S6 linkedPlanningPacket" '.linkedPlanningPacket' "null"
assert_stdout_json_value "S6 planningOnly" '.planningOnly' "false"
assert_stdout_json_value "S6 planningOnlyJustification" '.planningOnlyJustification' "null"
assert_stdout_json_value "S6 specDependsOn length" '(.specDependsOn | length)' "0"
assert_stdout_json_value "S6 certifiedAt" '.certifiedAt' "null"
assert_stdout_json_value "S6 requiresRevalidation" '.requiresRevalidation' "false"
assert_files_same "S6 dry-run source file" "$WORKSPACE/s6-before.json" "$state"

echo ""
echo "=== Selftest verdict ==="
printf '  Total assertions: %d\n' "$((PASS_COUNT + FAIL_COUNT))"
printf '  Passed:           %d\n' "$PASS_COUNT"
printf '  Failed:           %d\n' "$FAIL_COUNT"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
  echo "state-linkage-backfill-selftest: FAILED" >&2
  for scenario in "${FAILED_SCENARIOS[@]}"; do
    echo "  - $scenario" >&2
  done
  exit 1
fi

echo "state-linkage-backfill-selftest: PASSED"
exit 0