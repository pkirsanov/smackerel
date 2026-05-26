#!/usr/bin/env bash
set -euo pipefail

# Hermetic selftest for SCOPE-6 validation latency observability.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPORT="$SCRIPT_DIR/validation-latency-report.sh"

if [[ ! -f "$REPORT" ]]; then
  echo "validation-latency-report-selftest: report script not found at $REPORT" >&2
  exit 2
fi

WORKSPACE="$(mktemp -d -t bubbles-scope6-latency-selftest-XXXXXXXX)"
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

write_sample_session() {
  local repo="$1"
  cat > "$repo/.specify/memory/bubbles.session.json" <<'EOF'
{
  "sessionId": "latency-selftest-session",
  "executionHistory": [
    {
      "agent": "bubbles.implement",
      "phasesExecuted": ["implement"],
      "featureDir": "specs/900-latency-fixture",
      "runStartedAt": "2026-05-24T09:00:00Z",
      "runCompletedAt": "2026-05-24T09:20:00Z"
    },
    {
      "agent": "bubbles.test",
      "phasesExecuted": ["test"],
      "featureDir": "specs/900-latency-fixture",
      "runStartedAt": "2026-05-24T09:25:00Z",
      "runCompletedAt": "2026-05-24T09:35:00Z"
    },
    {
      "agent": "bubbles.validate",
      "phasesExecuted": ["validate"],
      "featureDir": "specs/002-other-feature",
      "runStartedAt": "2026-05-24T10:00:00Z",
      "runCompletedAt": "2026-05-24T10:05:00Z"
    },
    {
      "agent": "bubbles.audit",
      "phasesExecuted": ["audit"],
      "featureDir": "specs/900-latency-fixture",
      "runStartedAt": "2026-05-24T10:10:00Z",
      "runCompletedAt": "not-a-timestamp"
    },
    {
      "agent": "bubbles.docs",
      "phasesExecuted": ["docs"],
      "featureDir": "specs/900-latency-fixture",
      "runStartedAt": "2026-05-24T10:15:00Z"
    }
  ],
  "turnSnapshots": [
    {
      "agent": "bubbles.design",
      "phase": "design",
      "specDir": "specs/900-latency-fixture",
      "startedAt": "2026-05-24T08:00:00Z",
      "completedAt": "2026-05-24T08:07:00Z"
    }
  ]
}
EOF
}

run_report() {
  local repo="$1"
  shift
  set +e
  bash "$REPORT" --repo-root "$repo" --now "2026-05-24T12:00:00Z" "$@" > "$WORKSPACE/stdout.last" 2> "$WORKSPACE/stderr.last"
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
  if grep -qF -- "$needle" "$WORKSPACE/stdout.last"; then
    ok "$label stdout contains '$needle'"
  else
    ko "$label stdout missing '$needle'"
    cat "$WORKSPACE/stdout.last"
  fi
}

assert_stdout_not_contains() {
  local label="$1"
  local needle="$2"
  if grep -qF -- "$needle" "$WORKSPACE/stdout.last"; then
    ko "$label stdout unexpectedly contains '$needle'"
    cat "$WORKSPACE/stdout.last"
  else
    ok "$label stdout omits '$needle'"
  fi
}

assert_stderr_contains() {
  local label="$1"
  local needle="$2"
  if grep -qF -- "$needle" "$WORKSPACE/stderr.last"; then
    ok "$label stderr contains '$needle'"
  else
    ko "$label stderr missing '$needle'"
    cat "$WORKSPACE/stderr.last"
  fi
}

echo "=== validation-latency-report-selftest (SCOPE-6) ==="

echo ""
echo "--- S1: latency report renders phase percentiles ---"
repo="$(stage_repo s1-render)"
write_sample_session "$repo"
run_report "$repo" --since 7
assert_exit "S1 render" 0
assert_stdout_contains "S1" "| Phase |"
assert_stdout_contains "S1" "| implement | all | all | 1 | 20m0s | 20m0s | 20m0s | 30m0s | yes |"
assert_stdout_contains "S1" "Skipped records: 2"

echo ""
echo "--- S2: --spec filters rows to one spec ---"
repo="$(stage_repo s2-filter)"
write_sample_session "$repo"
run_report "$repo" --since 7 --spec specs/900-latency-fixture
assert_exit "S2 filter" 0
assert_stdout_contains "S2" "Spec filter: specs/900-latency-fixture"
assert_stdout_contains "S2" "| implement | all | specs/900-latency-fixture |"
assert_stdout_not_contains "S2" "specs/002-other-feature"

echo ""
echo "--- S3: grouping by phase-agent renders agent-specific rows ---"
repo="$(stage_repo s3-group)"
write_sample_session "$repo"
run_report "$repo" --since 7 --group phase-agent
assert_exit "S3 group" 0
assert_stdout_contains "S3" "Group: phase-agent"
assert_stdout_contains "S3" "| test | bubbles.test | all | 1 | 10m0s | 10m0s | 10m0s | 15m0s | yes |"

echo ""
echo "--- S4: empty history exits 0 with table header ---"
repo="$(stage_repo s4-empty)"
cat > "$repo/.specify/memory/bubbles.session.json" <<'EOF'
{
  "sessionId": "empty-latency-session",
  "executionHistory": [],
  "turnSnapshots": []
}
EOF
run_report "$repo" --since 7
assert_exit "S4 empty" 0
assert_stdout_contains "S4" "| Phase |"
assert_stdout_contains "S4" "No valid phase durations found"

echo ""
echo "--- S5: malformed JSON exits 2 ---"
repo="$(stage_repo s5-malformed-json)"
cat > "$repo/.specify/memory/bubbles.session.json" <<'EOF'
{ this is not json }
EOF
run_report "$repo" --since 7
assert_exit "S5 malformed JSON" 2
assert_stderr_contains "S5" "malformed JSON"

echo ""
echo "--- S6: missing session file exits 0 with empty report ---"
repo="$(stage_repo s6-missing)"
run_report "$repo" --since 7
assert_exit "S6 missing" 0
assert_stdout_contains "S6" "No session JSON found"

echo ""
echo "=== Selftest verdict ==="
printf '  Total assertions: %d\n' "$((PASS_COUNT + FAIL_COUNT))"
printf '  Passed:           %d\n' "$PASS_COUNT"
printf '  Failed:           %d\n' "$FAIL_COUNT"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
  echo "validation-latency-report-selftest: FAILED" >&2
  for scenario in "${FAILED_SCENARIOS[@]}"; do
    echo "  - $scenario" >&2
  done
  exit 1
fi

echo "validation-latency-report-selftest: PASSED"
exit 0
