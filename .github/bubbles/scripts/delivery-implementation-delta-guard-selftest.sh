#!/usr/bin/env bash
set -euo pipefail

# Hermetic selftest for Gate G093 - delivery_implementation_delta_gate.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/delivery-implementation-delta-guard.sh"

if [[ ! -x "$GUARD" ]]; then
  echo "delivery-implementation-delta-guard-selftest: guard not executable at $GUARD" >&2
  exit 2
fi

WORKSPACE_BASE="${HOME:-.}/.cache"
mkdir -p "$WORKSPACE_BASE"
WORKSPACE="$(mktemp -d -p "$WORKSPACE_BASE" bubbles-g093-selftest-XXXXXXXX)"
cleanup() {
  rm -rf "$WORKSPACE" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

PASS_COUNT=0
FAIL_COUNT=0
FAILED_SCENARIOS=()

ok() { PASS_COUNT=$((PASS_COUNT + 1)); printf '  PASS: %s\n' "$*"; }
ko() { FAIL_COUNT=$((FAIL_COUNT + 1)); FAILED_SCENARIOS+=("$*"); printf '  FAIL: %s\n' "$*"; }

write_workflows() {
  local repo="$1"
  mkdir -p "$repo/bubbles"
  cp "$SCRIPT_DIR/../workflows.yaml" "$repo/bubbles/workflows.yaml"
}

write_state() {
  local repo="$1"
  local mode="$2"
  local status="$3"
  local planning_only="$4"
  local justification_json="$5"
  mkdir -p "$repo/specs/100-current"
  cat > "$repo/specs/100-current/state.json" <<EOF
{
  "version": 3,
  "featureDir": "specs/100-current",
  "featureName": "G093 Fixture",
  "status": "$status",
  "workflowMode": "$mode",
  "planningOnly": $planning_only,
  "planningOnlyJustification": $justification_json,
  "certification": { "status": "$status" },
  "executionHistory": []
}
EOF
  cat > "$repo/specs/100-current/report.md" <<'EOF'
# Report

## Code Diff Evidence

No code diff evidence recorded yet.
EOF
}

stage_repo() {
  local sid="$1"
  local mode="${2:-full-delivery}"
  local status="${3:-in_progress}"
  local planning_only="${4:-false}"
  local justification_json="${5:-null}"
  local repo="$WORKSPACE/$sid"
  rm -rf "$repo"
  mkdir -p "$repo"
  write_workflows "$repo"
  write_state "$repo" "$mode" "$status" "$planning_only" "$justification_json"
  git -C "$repo" init -q
  git -C "$repo" config user.email "g093-selftest@example.invalid"
  git -C "$repo" config user.name "G093 Selftest"
  git -C "$repo" add .
  git -C "$repo" commit -m "baseline" -q
  printf '%s' "$repo"
}

commit_all() {
  local repo="$1"
  local msg="$2"
  git -C "$repo" add .
  git -C "$repo" commit -m "$msg" -q
}

run_guard() {
  local repo="$1"
  shift
  set +e
  BUBBLES_REPO_ROOT="$repo" bash "$GUARD" "$repo/specs/100-current" "$@" > "$WORKSPACE/stdout.last" 2> "$WORKSPACE/stderr.last"
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

echo "=== delivery-implementation-delta-guard-selftest (Gate G093) ==="

echo ""
echo "--- S1: done-ceiling delivery with spec-only diff fails ---"
repo="$(stage_repo s1-spec-only)"
base="$(git -C "$repo" rev-parse HEAD)"
mkdir -p "$repo/.specify/memory"
cat > "$repo/specs/100-current/spec.md" <<'EOF'
# Spec-only change
EOF
cat > "$repo/.specify/memory/agents.md" <<'EOF'
# Planning memory change
EOF
commit_all "$repo" "spec-only delivery"
head="$(git -C "$repo" rev-parse HEAD)"
run_guard "$repo" --base "$base" --head "$head"
assert_exit "S1 spec-only delivery" 1
assert_stderr_contains "S1" "G093"
assert_stderr_contains "S1" "planning-only"
assert_stderr_contains "S1" "nextOwner: implementation"
assert_stderr_contains "S1" "planning-only downgrade"

echo ""
echo "--- S2: done-ceiling delivery with source and test delta passes ---"
repo="$(stage_repo s2-source-test)"
base="$(git -C "$repo" rev-parse HEAD)"
mkdir -p "$repo/src" "$repo/tests/regression"
cat > "$repo/src/delivery_delta.ts" <<'EOF'
export const deliveryDelta = true;
EOF
cat > "$repo/tests/regression/test_delivery_delta.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo delivery-delta-regression
EOF
commit_all "$repo" "source and test delivery"
head="$(git -C "$repo" rev-parse HEAD)"
run_guard "$repo" --base "$base" --head "$head"
assert_exit "S2 source plus test" 0
assert_stdout_contains "S2" "PASS Gate G093"
assert_stdout_contains "S2" "source (1)"
assert_stdout_contains "S2" "test (1)"

echo ""
echo "--- S3: planning-only specs_hardened mode is exempt and leaves G087 responsible ---"
repo="$(stage_repo s3-planning-only spec-scope-hardening specs_hardened true '"Planning packet intentionally has no implementation target."')"
base="$(git -C "$repo" rev-parse HEAD)"
cat > "$repo/specs/100-current/design.md" <<'EOF'
# Hardened planning packet
EOF
commit_all "$repo" "planning-only packet"
head="$(git -C "$repo" rev-parse HEAD)"
run_guard "$repo" --base "$base" --head "$head"
assert_exit "S3 planning-only exemption" 0
assert_stdout_contains "S3" "SKIP Gate G093"
assert_stdout_contains "S3" "statusCeiling=specs_hardened"
assert_stdout_contains "S3" "G087 remains responsible"

echo ""
echo "--- S4: docs-only lower-ceiling mode is exempt from delivery delta ---"
repo="$(stage_repo s4-docs-only docs-only docs_updated false null)"
base="$(git -C "$repo" rev-parse HEAD)"
mkdir -p "$repo/docs"
cat > "$repo/docs/usage.md" <<'EOF'
# User-facing docs update
EOF
commit_all "$repo" "docs-only update"
head="$(git -C "$repo" rev-parse HEAD)"
run_guard "$repo" --base "$base" --head "$head"
assert_exit "S4 docs-only exemption" 0
assert_stdout_contains "S4" "SKIP Gate G093"
assert_stdout_contains "S4" "statusCeiling=docs_updated"
assert_stdout_contains "S4" "lower ceiling prevents done certification"

echo ""
echo "--- S5: report Code Diff Evidence classifies delivery paths when git window is unavailable ---"
repo="$(stage_repo s5-report-delivery)"
cat > "$repo/specs/100-current/report.md" <<'EOF'
# Report

### Code Diff Evidence

**Command:** git diff --name-only BASE HEAD
**Exit Code:** 0
**Claim Source:** executed
M src/report_delta.ts
M tests/regression/test_report_delta.sh

### Test Evidence

Not part of this fixture.
EOF
rm -rf "$repo/.git"
run_guard "$repo"
assert_exit "S5 report delivery evidence" 0
assert_stdout_contains "S5" "PASS Gate G093"
assert_stdout_contains "S5" "G053-compatible evidence source accepted"
assert_stdout_contains "S5" "reportCodeDiffSections=1"

repo="$(stage_repo s5-report-planning)"
cat > "$repo/specs/100-current/report.md" <<'EOF'
# Report

### Code Diff Evidence

**Command:** git diff --name-only BASE HEAD
**Exit Code:** 0
**Claim Source:** executed
M specs/100-current/spec.md
M .specify/memory/agents.md
EOF
rm -rf "$repo/.git"
run_guard "$repo"
assert_exit "S5 report planning-only evidence" 1
assert_stderr_contains "S5 planning" "G093"
assert_stderr_contains "S5 planning" "planning-only"

echo ""
echo "--- S6: blocked envelope includes changed-path classification and next owner ---"
repo="$(stage_repo s6-blocked-envelope)"
base="$(git -C "$repo" rev-parse HEAD)"
cat > "$repo/specs/100-current/scopes.md" <<'EOF'
# Scope-only update
EOF
commit_all "$repo" "scope-only delivery"
head="$(git -C "$repo" rev-parse HEAD)"
run_guard "$repo" --base "$base" --head "$head"
assert_exit "S6 blocked envelope" 1
assert_stderr_contains "S6" "RESULT-ENVELOPE"
assert_stderr_contains "S6" "changedPathClassification"
assert_stderr_contains "S6" "nextOwner: implementation"
assert_stderr_contains "S6" "alternateOwner: planning-only downgrade"

echo ""
echo "=== Selftest verdict ==="
printf '  Total assertions: %d\n' "$((PASS_COUNT + FAIL_COUNT))"
printf '  Passed:           %d\n' "$PASS_COUNT"
printf '  Failed:           %d\n' "$FAIL_COUNT"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
  echo "delivery-implementation-delta-guard-selftest: FAILED" >&2
  for scenario in "${FAILED_SCENARIOS[@]}"; do
    echo "  - $scenario" >&2
  done
  exit 1
fi

echo "delivery-implementation-delta-guard-selftest: PASSED"
exit 0