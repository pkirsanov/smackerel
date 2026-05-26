#!/usr/bin/env bash
set -euo pipefail

# Hermetic selftest for SCOPE-7 retro convergence health (Gate G090).

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RETRO_HEALTH="$SCRIPT_DIR/retro-convergence-health.sh"

if [[ ! -f "$RETRO_HEALTH" ]]; then
  echo "retro-convergence-health-selftest: script not found at $RETRO_HEALTH" >&2
  exit 2
fi

WORKSPACE="$(mktemp -d -t bubbles-scope7-retro-health-selftest-XXXXXXXX)"
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

write_healthy_session() {
  local repo="$1"
  cat > "$repo/.specify/memory/bubbles.session.json" <<'EOF'
{
  "sessionId": "retro-health-healthy-session",
  "convergenceLoops": [
    {"specDir": "specs/900-retro-fixture", "agent": "bubbles.workflow", "iterationCount": 1},
    {"specDir": "specs/900-retro-fixture", "agent": "bubbles.workflow", "iterationCount": 2}
  ],
  "envelopesReceived": [
    {"specDir": "specs/900-retro-fixture", "rawSizeBytes": 1200, "compactedAt": "2026-05-24T10:00:00Z"},
    {"specDir": "specs/900-retro-fixture", "rawSizeBytes": 800}
  ],
  "turnSnapshots": [
    {"specDir": "specs/900-retro-fixture", "turnNumber": 1, "startedAt": "2026-05-24T09:00:00Z", "completedAt": "2026-05-24T09:05:00Z", "content": "implementation progress"},
    {"specDir": "specs/900-retro-fixture", "turnNumber": 2, "startedAt": "2026-05-24T09:06:00Z", "completedAt": "2026-05-24T09:10:00Z", "content": "validation progress"}
  ],
  "messages": [
    {"specDir": "specs/900-retro-fixture", "role": "assistant", "content": "continue implementation"}
  ]
}
EOF
}

write_state_snapshot_pair_session() {
  local repo="$1"
  cat > "$repo/.specify/memory/bubbles.session.json" <<'EOF'
{
  "sessionId": "retro-health-state-snapshot-pair-session",
  "turnSnapshots": [
    {"specDir": "specs/900-retro-fixture", "turnNumber": 1, "timestamp": "2026-05-24T17:31:49Z", "phase": "simplify", "scopeId": null, "mode": "start", "note": "turn start", "agent": "bubbles.simplify"},
    {"specDir": "specs/900-retro-fixture", "turnNumber": 2, "timestamp": "2026-05-24T17:49:23Z", "phase": "simplify", "scopeId": null, "mode": "end", "note": "turn end", "agent": "bubbles.simplify"}
  ]
}
EOF
}

write_state_snapshot_single_sided_session() {
  local repo="$1"
  cat > "$repo/.specify/memory/bubbles.session.json" <<'EOF'
{
  "sessionId": "retro-health-state-snapshot-single-sided-session",
  "turnSnapshots": [
    {"specDir": "specs/900-retro-fixture", "turnNumber": 1, "timestamp": "2026-05-24T17:31:49Z", "phase": "simplify", "scopeId": null, "mode": "start", "note": "turn start", "agent": "bubbles.simplify"}
  ]
}
EOF
}

write_snapshot_breach_session() {
  local repo="$1"
  cat > "$repo/.specify/memory/bubbles.session.json" <<'EOF'
{
  "sessionId": "retro-health-snapshot-breach-session",
  "turnSnapshots": [
    {"specDir": "specs/900-retro-fixture", "turnNumber": 1, "startedAt": "2026-05-24T09:00:00Z", "completedAt": "2026-05-24T09:05:00Z"},
    {"specDir": "specs/900-retro-fixture", "turnNumber": 2, "startedAt": "2026-05-24T09:06:00Z"}
  ]
}
EOF
}

write_p0_recap_handoff_session() {
  local repo="$1"
  cat > "$repo/.specify/memory/bubbles.session.json" <<'EOF'
{
  "sessionId": "retro-health-p0-recap-handoff-session",
  "turnSnapshots": [
    {"specDir": "specs/900-retro-fixture", "turnNumber": 1, "startedAt": "2026-05-24T09:00:00Z", "completedAt": "2026-05-24T09:05:00Z", "content": "recap requested"},
    {"specDir": "specs/900-retro-fixture", "turnNumber": 2, "startedAt": "2026-05-24T09:06:00Z", "completedAt": "2026-05-24T09:07:00Z", "content": "handoff requested"},
    {"specDir": "specs/900-retro-fixture", "turnNumber": 3, "startedAt": "2026-05-24T09:08:00Z", "completedAt": "2026-05-24T09:09:00Z", "content": "another handoff requested"}
  ]
}
EOF
}

run_health() {
  local repo="$1"
  shift
  set +e
  bash "$RETRO_HEALTH" specs/900-retro-fixture --repo-root "$repo" "$@" > "$WORKSPACE/stdout.last" 2> "$WORKSPACE/stderr.last"
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

assert_json() {
  local label="$1"
  if jq -e . "$WORKSPACE/stdout.last" >/dev/null 2>&1; then
    ok "$label stdout is valid JSON"
  else
    ko "$label stdout is not valid JSON"
    cat "$WORKSPACE/stdout.last"
  fi
}

assert_jq() {
  local label="$1"
  local expr="$2"
  if jq -e "$expr" "$WORKSPACE/stdout.last" >/dev/null 2>&1; then
    ok "$label jq '$expr'"
  else
    ko "$label jq failed '$expr'"
    cat "$WORKSPACE/stdout.last"
  fi
}

echo "=== retro-convergence-health-selftest (SCOPE-7 / G090) ==="

echo ""
echo "--- S1: legacy health metrics computed and JSON parses ---"
repo="$(stage_repo s1-metrics)"
write_healthy_session "$repo"
run_health "$repo" --schema legacy
assert_exit "S1 legacy metrics" 0
assert_json "S1 legacy metrics"
assert_jq "S1 legacy exact keys" 'keys == ["avgLoopIterations", "compactionFrequency", "maxConvergenceIterations", "preExistingDeferralCount", "snapshotCompleteness"]'
assert_jq "S1 average loop count" '.avgLoopIterations == 1.5'

echo ""
echo "--- S1b: full schema includes convergenceHealth output contract ---"
repo="$(stage_repo s1b-full-schema)"
write_healthy_session "$repo"
run_health "$repo"
assert_exit "S1b full schema" 0
assert_json "S1b full schema"
assert_jq "S1b convergenceHealth schema" '.convergenceHealth == {"recapCount": 0, "handoffCount": 0, "summarizeHistoryCount": 0, "turnCount": 2, "slo": "pass"}'

echo ""
echo "--- S1c: paired state-snapshot records count as one complete snapshot ---"
repo="$(stage_repo s1c-state-snapshot-pair)"
write_state_snapshot_pair_session "$repo"
run_health "$repo"
assert_exit "S1c state-snapshot pair" 0
assert_json "S1c state-snapshot pair"
assert_jq "S1c snapshot completeness" '.snapshotCompleteness == 1'
assert_jq "S1c SLO pass" '.convergenceHealth.slo == "pass"'

echo ""
echo "--- S2: snapshotCompleteness breach exits 1 and cites G090 ---"
repo="$(stage_repo s2-snapshot-breach)"
write_snapshot_breach_session "$repo"
run_health "$repo"
assert_exit "S2 snapshot breach" 1
assert_stderr_contains "S2" "G090"
assert_stderr_contains "S2" "snapshotCompleteness"

echo ""
echo "--- S2a: single-sided state-snapshot record still fails G090 ---"
repo="$(stage_repo s2a-state-snapshot-single-sided)"
write_state_snapshot_single_sided_session "$repo"
run_health "$repo"
assert_exit "S2a state-snapshot single-sided" 1
assert_stderr_contains "S2a" "G090"
assert_stderr_contains "S2a" "snapshotCompleteness=0"

echo ""
echo "--- S2b: more than two recap/handoff invocations is P0 failed ---"
repo="$(stage_repo s2b-p0-recap-handoff)"
write_p0_recap_handoff_session "$repo"
run_health "$repo"
assert_exit "S2b recap/handoff P0" 1
assert_stderr_contains "S2b" "P0 convergence regression"
assert_stderr_contains "S2b" "recapHandoffInvocationCount=3"

echo ""
echo "--- S3: all thresholds healthy emits markdown section ---"
repo="$(stage_repo s3-markdown)"
write_healthy_session "$repo"
run_health "$repo" --format markdown
assert_exit "S3 markdown" 0
assert_stdout_contains "S3" "## Convergence Health"
assert_stdout_contains "S3" 'SLO: `pass`'

echo ""
echo "--- S3b: --out writes markdown while stdout remains JSON ---"
repo="$(stage_repo s3b-out)"
write_healthy_session "$repo"
out_path="$WORKSPACE/retro-health.md"
run_health "$repo" --out "$out_path"
assert_exit "S3b --out" 0
assert_json "S3b stdout JSON"
if [[ -f "$out_path" ]] && grep -qF "## Convergence Health" "$out_path"; then
  ok "S3b --out markdown file contains Convergence Health"
else
  ko "S3b --out markdown file missing Convergence Health"
  [[ -f "$out_path" ]] && cat "$out_path"
fi

echo ""
echo "=== Selftest verdict ==="
printf '  Total assertions: %d\n' "$((PASS_COUNT + FAIL_COUNT))"
printf '  Passed:           %d\n' "$PASS_COUNT"
printf '  Failed:           %d\n' "$FAIL_COUNT"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
  echo "retro-convergence-health-selftest: FAILED" >&2
  for scenario in "${FAILED_SCENARIOS[@]}"; do
    echo "  - $scenario" >&2
  done
  exit 1
fi

echo "retro-convergence-health-selftest: PASSED"
exit 0