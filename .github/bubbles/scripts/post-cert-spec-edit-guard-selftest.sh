#!/usr/bin/env bash
set -euo pipefail

# Hermetic selftest for Gate G088 - post_certification_spec_edit_gate.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/post-cert-spec-edit-guard.sh"

if [[ ! -x "$GUARD" ]]; then
  echo "post-cert-spec-edit-guard-selftest: guard not executable at $GUARD" >&2
  exit 2
fi

WORKSPACE="$(mktemp -d -t bubbles-g088-selftest-XXXXXXXX)"
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
  mkdir -p "$repo/specs/300-certified/scopes/03-example"
  git -C "$repo" -c init.defaultBranch=main init >/dev/null
  git -C "$repo" config user.email selftest@example.com
  git -C "$repo" config user.name selftest
  printf '%s' "$repo"
}

commit_all() {
  local repo="$1"
  local when="$2"
  local message="$3"
  git -C "$repo" add .
  GIT_AUTHOR_DATE="$when" GIT_COMMITTER_DATE="$when" git -C "$repo" commit -m "$message" >/dev/null
}

write_truth_files() {
  local repo="$1"
  mkdir -p "$repo/specs/300-certified/scopes/03-example"
  cat > "$repo/specs/300-certified/spec.md" <<'EOF'
# Certified Fixture Spec

Initial planning truth.
EOF
  cat > "$repo/specs/300-certified/design.md" <<'EOF'
# Certified Fixture Design

Initial design truth.
EOF
  cat > "$repo/specs/300-certified/scopes/_index.md" <<'EOF'
# Scope Index

| # | Scope | Status |
|---|-------|--------|
| 03 | example | Done |
EOF
  cat > "$repo/specs/300-certified/scopes/03-example/scope.md" <<'EOF'
# Scope 03 Example

**Status:** Done
EOF
}

write_state() {
  local repo="$1"
  local status="$2"
  local certified_at_json="$3"
  local requires_revalidation="$4"
  local execution_history_json="$5"
  local extra_fields="${6:-}"
  cat > "$repo/specs/300-certified/state.json" <<EOF
{
  "version": 3,
  "featureDir": "specs/300-certified",
  "featureName": "Certified Fixture",
  "status": "$status",
  "workflowMode": "full-delivery",
  "linkedImplementationSpec": null,
  "linkedPlanningPacket": null,
  "planningOnly": false,
  "planningOnlyJustification": null,
  "specDependsOn": [],
  "certifiedAt": $certified_at_json,
  "requiresRevalidation": $requires_revalidation,
  "executionHistory": $execution_history_json$extra_fields
}
EOF
}

run_guard() {
  local repo="$1"
  set +e
  bash "$GUARD" "$repo/specs/300-certified" > "$WORKSPACE/stdout.last" 2> "$WORKSPACE/stderr.last"
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

echo "=== post-cert-spec-edit-guard-selftest (Gate G088) ==="

echo ""
echo "--- S0: clean done spec passes ---"
repo="$(stage_repo s0-clean-done)"
write_truth_files "$repo"
write_state "$repo" "done" '"2026-05-01T00:00:00Z"' "false" '[]'
commit_all "$repo" "2026-04-30T00:00:00Z" "baseline certified spec"
run_guard "$repo"
assert_exit "S0 clean done" 0
assert_stdout_contains "S0" "PASS Gate G088"
assert_stdout_contains "S0" "post_certification_spec_edit_gate"

echo ""
echo "--- S1: done spec edited after certification blocks ---"
repo="$(stage_repo s1-post-cert-edit)"
write_truth_files "$repo"
write_state "$repo" "done" '"2026-05-01T00:00:00Z"' "false" '[]'
commit_all "$repo" "2026-04-30T00:00:00Z" "baseline certified spec"
printf '\nPost-certification edit.\n' >> "$repo/specs/300-certified/spec.md"
commit_all "$repo" "2026-05-02T00:00:00Z" "post-cert spec edit"
run_guard "$repo"
assert_exit "S1 post-cert edit" 1
assert_stderr_contains "S1" "G088"
assert_stderr_contains "S1" "post_certification_spec_edit_gate"
assert_stderr_contains "S1" "post-cert spec edit"
assert_stderr_contains "S1" "specs/300-certified/spec.md"

echo ""
echo "--- S1b: resetting the post-cert edit restores pass ---"
git -C "$repo" reset --hard HEAD~1 >/dev/null
run_guard "$repo"
assert_exit "S1b reset restores pass" 0
assert_stdout_contains "S1b" "PASS Gate G088"

echo ""
echo "--- S2: demoted spec can be edited ---"
repo="$(stage_repo s2-demoted)"
write_truth_files "$repo"
write_state "$repo" "in_progress" '"2026-05-01T00:00:00Z"' "false" '[]'
commit_all "$repo" "2026-04-30T00:00:00Z" "baseline demoted spec"
printf '\nAllowed demoted edit.\n' >> "$repo/specs/300-certified/spec.md"
commit_all "$repo" "2026-05-02T00:00:00Z" "demoted spec edit"
run_guard "$repo"
assert_exit "S2 demoted edit" 0
assert_stdout_contains "S2" "status=in_progress is not certified done"

echo ""
echo "--- S3: current spec-review recertification permits prior edit ---"
repo="$(stage_repo s3-recertified)"
write_truth_files "$repo"
write_state "$repo" "done" '"2026-05-04T00:00:00Z"' "false" '[{"agent":"bubbles.spec-review","reviewStatus":"CURRENT","runCompletedAt":"2026-05-03T00:00:00Z"}]'
commit_all "$repo" "2026-04-30T00:00:00Z" "baseline certified spec"
printf '\nReviewed planning update.\n' >> "$repo/specs/300-certified/design.md"
commit_all "$repo" "2026-05-02T00:00:00Z" "reviewed design update"
run_guard "$repo"
assert_exit "S3 recertified current" 0
assert_stdout_contains "S3" "PASS Gate G088"
assert_stdout_contains "S3" "currentSpecReview=2026-05-03T00:00:00Z"

echo ""
echo "--- S4: per-scope planning edit after certification blocks ---"
repo="$(stage_repo s4-scope-edit)"
write_truth_files "$repo"
write_state "$repo" "done" '"2026-05-01T00:00:00Z"' "false" '[]'
commit_all "$repo" "2026-04-30T00:00:00Z" "baseline certified spec"
printf '\nPer-scope post-cert edit.\n' >> "$repo/specs/300-certified/scopes/03-example/scope.md"
commit_all "$repo" "2026-05-02T00:00:00Z" "post-cert scope edit"
run_guard "$repo"
assert_exit "S4 per-scope edit" 1
assert_stderr_contains "S4" "G088"
assert_stderr_contains "S4" "specs/300-certified/scopes/03-example/scope.md"

echo ""
echo "--- S5: done spec missing certifiedAt fails as runtime error ---"
repo="$(stage_repo s5-missing-certified-at)"
write_truth_files "$repo"
write_state "$repo" "done" "null" "false" '[]'
commit_all "$repo" "2026-04-30T00:00:00Z" "baseline missing certifiedAt"
run_guard "$repo"
assert_exit "S5 missing certifiedAt" 2
assert_stderr_contains "S5" "certifiedAt"

echo ""
echo "--- S6: requiresRevalidation flag allows certified edit to stay visible ---"
repo="$(stage_repo s6-requires-revalidation)"
write_truth_files "$repo"
write_state "$repo" "done" '"2026-05-01T00:00:00Z"' "true" '[]'
commit_all "$repo" "2026-04-30T00:00:00Z" "baseline certified spec"
printf '\nFlagged revalidation edit.\n' >> "$repo/specs/300-certified/spec.md"
commit_all "$repo" "2026-05-02T00:00:00Z" "flagged post-cert edit"
run_guard "$repo"
assert_exit "S6 requiresRevalidation" 0
assert_stdout_contains "S6" "requiresRevalidation=true"
assert_stdout_contains "S6" "postCertEdits=1"

echo ""
echo "--- S7: legacy read-only done_with_concerns clean spec passes ---"
repo="$(stage_repo s7-legacy-done-with-concerns)"
write_truth_files "$repo"
write_state "$repo" "done_with_concerns" '"2026-05-01T00:00:00Z"' "false" '[]' ',"legacyStatusCompatibility":true'
commit_all "$repo" "2026-04-30T00:00:00Z" "baseline legacy done_with_concerns spec"
run_guard "$repo"
assert_exit "S7 legacy read-only done_with_concerns" 0
assert_stdout_contains "S7" "PASS Gate G088"
assert_stdout_contains "S7" "status=done_with_concerns"

echo ""
echo "--- S8: touched legacy done_with_concerns spec blocks until migration ---"
repo="$(stage_repo s8-touched-legacy-done-with-concerns)"
write_truth_files "$repo"
write_state "$repo" "done_with_concerns" '"2026-05-01T00:00:00Z"' "false" '[]' ',"legacyStatusCompatibility":true'
commit_all "$repo" "2026-04-30T00:00:00Z" "baseline legacy done_with_concerns spec"
printf '\nTouched legacy planning truth.\n' >> "$repo/specs/300-certified/design.md"
commit_all "$repo" "2026-05-02T00:00:00Z" "touched legacy design update"
run_guard "$repo"
assert_exit "S8 touched legacy done_with_concerns" 1
assert_stderr_contains "S8" "G088"
assert_stderr_contains "S8" "G092"
assert_stderr_contains "S8" "done plus observations or blocked"

echo ""
echo "=== Selftest verdict ==="
printf '  Total assertions: %d\n' "$((PASS_COUNT + FAIL_COUNT))"
printf '  Passed:           %d\n' "$PASS_COUNT"
printf '  Failed:           %d\n' "$FAIL_COUNT"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
  echo "post-cert-spec-edit-guard-selftest: FAILED" >&2
  for scenario in "${FAILED_SCENARIOS[@]}"; do
    echo "  - $scenario" >&2
  done
  exit 1
fi

echo "post-cert-spec-edit-guard-selftest: PASSED"
exit 0