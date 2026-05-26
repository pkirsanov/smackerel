#!/usr/bin/env bash
set -euo pipefail

# Hermetic selftest for Gate G089 - inter_spec_dependency_gate.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/inter-spec-dependency-guard.sh"
HELPER="$SCRIPT_DIR/inter-spec-dependency-revalidation.sh"

if [[ ! -x "$GUARD" ]]; then
  echo "inter-spec-dependency-guard-selftest: guard not executable at $GUARD" >&2
  exit 2
fi

if [[ ! -x "$HELPER" ]]; then
  echo "inter-spec-dependency-guard-selftest: helper not executable at $HELPER" >&2
  exit 2
fi

WORKSPACE="$(mktemp -d -t bubbles-g089-selftest-XXXXXXXX)"
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
  mkdir -p "$repo/specs"
  printf '%s' "$repo"
}

write_state() {
  local repo="$1"
  local spec_rel="$2"
  local status="$3"
  local requires_revalidation="$4"
  local deps_json="$5"
  local certified_at_json="${6:-null}"
  local extra_fields="${7:-}"
  mkdir -p "$repo/$spec_rel"
  cat > "$repo/$spec_rel/state.json" <<EOF
{
  "version": 3,
  "featureDir": "$spec_rel",
  "featureName": "G089 Fixture",
  "status": "$status",
  "workflowMode": "full-delivery",
  "linkedImplementationSpec": null,
  "linkedPlanningPacket": null,
  "planningOnly": false,
  "planningOnlyJustification": null,
  "specDependsOn": $deps_json,
  "certifiedAt": $certified_at_json,
  "requiresRevalidation": $requires_revalidation,
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

run_helper() {
  local repo="$1"
  local spec_rel="$2"
  set +e
  bash "$HELPER" "$repo/$spec_rel" --repo-root "$repo" > "$WORKSPACE/stdout.last" 2> "$WORKSPACE/stderr.last"
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

assert_jq() {
  local label="$1"
  local file="$2"
  local expr="$3"
  if jq -e "$expr" "$file" >/dev/null; then
    ok "$label"
  else
    ko "$label"
    cat "$file"
  fi
}

echo "=== inter-spec-dependency-guard-selftest (Gate G089) ==="

echo ""
echo "--- S0: no dependencies pass ---"
repo="$(stage_repo s0-no-deps)"
write_state "$repo" "specs/100-dependent" "in_progress" "false" '[]'
run_guard "$repo" "specs/100-dependent"
assert_exit "S0 no deps" 0
assert_stdout_contains "S0" "PASS Gate G089"
assert_stdout_contains "S0" "dependencies=0"

echo ""
echo "--- S1: dependency on done spec passes ---"
repo="$(stage_repo s1-done-dependency)"
write_state "$repo" "specs/100-dependent" "in_progress" "false" '["specs/010-upstream"]'
write_state "$repo" "specs/010-upstream" "done" "false" '[]'
run_guard "$repo" "specs/100-dependent"
assert_exit "S1 done dependency" 0
assert_stdout_contains "S1" "PASS Gate G089"
assert_stdout_contains "S1" "specs/010-upstream:done"

echo ""
echo "--- S2: blocked dependency fails ---"
repo="$(stage_repo s2-blocked-dependency)"
write_state "$repo" "specs/100-dependent" "in_progress" "false" '["specs/011-unfinished"]'
write_state "$repo" "specs/011-unfinished" "blocked" "false" '[]'
run_guard "$repo" "specs/100-dependent"
assert_exit "S2 blocked dependency" 1
assert_stderr_contains "S2" "G089"
assert_stderr_contains "S2" "invalid dependency status 'blocked'"
assert_stderr_contains "S2" "specs/011-unfinished"

echo ""
echo "--- S3: legacy read-only done_with_concerns dependency is accepted ---"
repo="$(stage_repo s3-done-with-concerns)"
write_state "$repo" "specs/100-dependent" "in_progress" "false" '["specs/012-reviewed"]'
write_state "$repo" "specs/012-reviewed" "done_with_concerns" "false" '[]' '"2026-05-01T00:00:00Z"' ',"legacyStatusCompatibility":true'
run_guard "$repo" "specs/100-dependent"
assert_exit "S3 legacy read-only done_with_concerns dependency" 0
assert_stdout_contains "S3" "PASS Gate G089"
assert_stdout_contains "S3" "specs/012-reviewed:done_with_concerns(legacy-read-only)"

echo ""
echo "--- S3b: new done_with_concerns dependency is rejected by G092 boundary ---"
repo="$(stage_repo s3b-new-done-with-concerns)"
write_state "$repo" "specs/100-dependent" "in_progress" "false" '["specs/012-reviewed"]'
write_state "$repo" "specs/012-reviewed" "done_with_concerns" "false" '[]' '"2026-05-01T00:00:00Z"' ',"certification":{"status":"done_with_concerns","proposedStatus":"done_with_concerns"}'
run_guard "$repo" "specs/100-dependent"
assert_exit "S3b new done_with_concerns dependency" 1
assert_stderr_contains "S3b" "G089"
assert_stderr_contains "S3b" "G092"
assert_stderr_contains "S3b" "legacy read-only"

echo ""
echo "--- S4: demoted dependency cascades requiresRevalidation ---"
repo="$(stage_repo s4-demotion-revalidation)"
write_state "$repo" "specs/100-dependent" "in_progress" "false" '["specs/013-demoted"]'
write_state "$repo" "specs/013-demoted" "in_progress" "false" '[]'
run_guard "$repo" "specs/100-dependent"
assert_exit "S4 before helper demoted dependency" 1
assert_stderr_contains "S4 before helper" "invalid dependency status 'in_progress'"
run_helper "$repo" "specs/013-demoted"
assert_exit "S4 helper" 0
assert_stdout_contains "S4 helper" "markedDependents=1"
assert_jq "S4 dependent requiresRevalidation true" "$repo/specs/100-dependent/state.json" '.requiresRevalidation == true'
run_guard "$repo" "specs/100-dependent"
assert_exit "S4 after helper acknowledged revalidation" 0
assert_stdout_contains "S4 after helper" "requiresRevalidation=true"
assert_stdout_contains "S4 after helper" "acknowledgedUnstableDependencies=1"

echo ""
echo "--- S5: circular dependency fails ---"
repo="$(stage_repo s5-cycle)"
write_state "$repo" "specs/100-a" "done" "false" '["specs/200-b"]'
write_state "$repo" "specs/200-b" "done" "false" '["specs/100-a"]'
run_guard "$repo" "specs/100-a"
assert_exit "S5 dependency cycle" 1
assert_stderr_contains "S5" "G089"
assert_stderr_contains "S5" "dependency cycle detected"
assert_stderr_contains "S5" "specs/100-a -> specs/200-b -> specs/100-a"

echo ""
echo "--- S6: done spec requiring revalidation cannot stay certified ---"
repo="$(stage_repo s6-certified-revalidation-block)"
write_state "$repo" "specs/100-dependent" "done" "true" '["specs/010-upstream"]'
write_state "$repo" "specs/010-upstream" "done" "false" '[]'
run_guard "$repo" "specs/100-dependent"
assert_exit "S6 certified requiresRevalidation" 1
assert_stderr_contains "S6" "requiresRevalidation:true is unresolved"
assert_stderr_contains "S6" "demote the spec or recertify"

echo ""
echo "=== Selftest verdict ==="
printf '  Total assertions: %d\n' "$((PASS_COUNT + FAIL_COUNT))"
printf '  Passed:           %d\n' "$PASS_COUNT"
printf '  Failed:           %d\n' "$FAIL_COUNT"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
  echo "inter-spec-dependency-guard-selftest: FAILED" >&2
  for scenario in "${FAILED_SCENARIOS[@]}"; do
    echo "  - $scenario" >&2
  done
  exit 1
fi

echo "inter-spec-dependency-guard-selftest: PASSED"
exit 0