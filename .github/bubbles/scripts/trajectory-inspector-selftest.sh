#!/usr/bin/env bash
# trajectory-inspector-selftest.sh
#
# Hermetic selftest for trajectory-inspector.sh.
#
# Stages a synthetic Bubbles repo with:
#   - .specify/memory/bubbles.session.json (a session with 2 turn snapshots)
#   - .specify/memory/lessons.md (some lessons)
#   - specs/777-fake/state.json (one active spec)
#   - specs/777-fake/scopes.md (with Status: Done lines)
#
# Asserts:
#   - text format prints expected sections and fixture values
#   - --format json emits valid JSON containing sessionFound=true and the
#     two turnSnapshots
#   - "no active session" path emits text "(no active session)" and JSON
#     sessionFound=false, both with exit 0
#   - --last 1 trims turnSnapshots to 1 in JSON output
#   - usage error (--last bogus) exits 2
#   - --health reads JSON input and re-derives metrics from session state

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSPECTOR="$SCRIPT_DIR/trajectory-inspector.sh"
RETRO_HEALTH="$SCRIPT_DIR/retro-convergence-health.sh"

if [[ ! -f "$INSPECTOR" ]]; then
  echo "[selftest trajectory-inspector] FAIL: missing $INSPECTOR" >&2
  exit 1
fi

if [[ ! -f "$RETRO_HEALTH" ]]; then
  echo "[selftest trajectory-inspector] FAIL: missing $RETRO_HEALTH" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "[selftest trajectory-inspector] FAIL: jq is required for selftest" >&2
  exit 1
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

failures=0
pass() { echo "  PASS: $1"; }
fail() { echo "  FAIL: $1"; failures=$((failures + 1)); }

# --- Stage active-session fixture ----------------------------------------

repo="$TMPDIR/repo-active"
mkdir -p "$repo/.specify/memory" "$repo/specs/777-fake/scopes/01-scope-a" "$repo/specs/777-fake/scopes/02-scope-b"

cat > "$repo/.specify/memory/bubbles.session.json" <<'EOF'
{
  "sessionId": "selftest-session-001",
  "agent": "bubbles.implement",
  "featureDir": "specs/777-fake",
  "mode": "implement",
  "status": "in_progress",
  "currentPhase": "implement",
  "lastUpdatedAt": "2026-05-09T12:00:00Z",
  "turnSnapshots": [
    {
      "turnNumber": 1,
      "timestamp": "2026-05-09T11:00:00Z",
      "phase": "design",
      "scopeId": "01-scope-a",
      "mode": "implement",
      "agent": "bubbles.design",
      "note": "design recap started"
    },
    {
      "turnNumber": 2,
      "timestamp": "2026-05-09T11:30:00Z",
      "phase": "implement",
      "scopeId": "02-scope-b",
      "mode": "implement",
      "agent": "bubbles.implement",
      "note": "implement handoff for scope B"
    }
  ],
  "envelopesReceived": [
    {
      "specDir": "specs/777-fake",
      "rawSizeBytes": 1200,
      "compactedAt": "2026-05-09T11:45:00Z"
    },
    {
      "specDir": "specs/777-fake",
      "rawSizeBytes": 700
    }
  ],
  "executionHistory": [
    {
      "agent": "bubbles.design",
      "phasesExecuted": ["design"],
      "featureDir": "specs/777-fake",
      "runStartedAt": "2026-05-09T10:00:00Z",
      "runCompletedAt": "2026-05-09T10:05:00Z"
    },
    {
      "agent": "bubbles.implement",
      "phasesExecuted": ["implement"],
      "featureDir": "specs/777-fake",
      "runStartedAt": "2026-05-09T10:10:00Z",
      "runCompletedAt": "2026-05-09T10:25:00Z"
    }
  ]
}
EOF

cat > "$repo/.specify/memory/lessons.md" <<'EOF'
# Bubbles Lessons

- 2026-05-09: Always capture RED proof before changing implementation.
- 2026-05-09: Selftests must be hermetic.
EOF

cat > "$repo/specs/777-fake/state.json" <<'EOF'
{
  "spec": "777-fake",
  "status": "in_progress",
  "workflowMode": "implement",
  "execution": {
    "currentPhase": "implement",
    "currentScope": "02-scope-b",
    "completedScopes": ["01-scope-a"]
  }
}
EOF

cat > "$repo/specs/777-fake/scopes/01-scope-a/scope.md" <<'EOF'
# Scope A
Status: Done
EOF

cat > "$repo/specs/777-fake/scopes/02-scope-b/scope.md" <<'EOF'
# Scope B
Status: In Progress
EOF

# --- Test 1: text format active session ----------------------------------

set +e
text_log="$TMPDIR/text.log"
bash "$INSPECTOR" --repo-root "$repo" >"$text_log" 2>&1
text_rc=$?
set -e

if [[ "$text_rc" -eq 0 ]]; then
  pass "text format exits 0 (active session)"
else
  fail "text format expected exit 0, got $text_rc"
  sed -n '1,80p' "$text_log"
fi

for token in \
  "Bubbles Trajectory Inspector" \
  "1. Session Summary" \
  "2. Phase Progression" \
  "3. Scope Progression" \
  "4. Recent Lessons" \
  "5. Active Specs" \
  "selftest-session-001" \
  "777-fake" \
  "implement" \
  "completedScopes" \
  "01-scope-a" \
  "02-scope-b"
do
  if grep -Fq "$token" "$text_log"; then
    pass "text output contains '$token'"
  else
    fail "text output missing token: '$token'"
    sed -n '1,120p' "$text_log"
  fi
done

# --- Test 2: JSON format active session ----------------------------------

set +e
json_log="$TMPDIR/json.log"
bash "$INSPECTOR" --repo-root "$repo" --format json >"$json_log" 2>&1
json_rc=$?
set -e

if [[ "$json_rc" -eq 0 ]]; then
  pass "json format exits 0"
else
  fail "json format expected exit 0, got $json_rc"
  sed -n '1,40p' "$json_log"
fi

if jq -e . "$json_log" >/dev/null 2>&1; then
  pass "json output is valid JSON"
else
  fail "json output is not valid JSON"
  sed -n '1,40p' "$json_log"
fi

session_found="$(jq -r '.sessionFound' "$json_log" 2>/dev/null || echo unknown)"
if [[ "$session_found" == "true" ]]; then
  pass "json sessionFound=true"
else
  fail "json sessionFound expected true, got '$session_found'"
fi

snap_count="$(jq -r '.turnSnapshots | length' "$json_log" 2>/dev/null || echo 0)"
if [[ "$snap_count" == "2" ]]; then
  pass "json turnSnapshots length=2"
else
  fail "json turnSnapshots length expected 2, got '$snap_count'"
fi

# --- Test 3: --last 1 trims snapshots ------------------------------------

set +e
last1_log="$TMPDIR/last1.log"
bash "$INSPECTOR" --repo-root "$repo" --format json --last 1 >"$last1_log" 2>&1
last1_rc=$?
set -e

if [[ "$last1_rc" -eq 0 ]]; then
  pass "--last 1 exits 0"
else
  fail "--last 1 expected exit 0, got $last1_rc"
fi

last1_count="$(jq -r '.turnSnapshots | length' "$last1_log" 2>/dev/null || echo 0)"
if [[ "$last1_count" == "1" ]]; then
  pass "--last 1 trimmed turnSnapshots to 1"
else
  fail "--last 1 expected 1 snapshot, got '$last1_count'"
fi

last1_turn="$(jq -r '.turnSnapshots[0].turnNumber' "$last1_log" 2>/dev/null || echo 0)"
if [[ "$last1_turn" == "2" ]]; then
  pass "--last 1 kept the most recent turn (turnNumber=2)"
else
  fail "--last 1 kept wrong turn (expected 2, got '$last1_turn')"
fi

# --- Test 4: no-active-session path --------------------------------------

empty_repo="$TMPDIR/repo-empty"
mkdir -p "$empty_repo/.specify/memory"

set +e
empty_text="$TMPDIR/empty-text.log"
bash "$INSPECTOR" --repo-root "$empty_repo" >"$empty_text" 2>&1
empty_text_rc=$?
set -e

if [[ "$empty_text_rc" -eq 0 ]]; then
  pass "no-active-session text format exits 0"
else
  fail "no-active-session text expected exit 0, got $empty_text_rc"
fi

if grep -Fq "(no active session)" "$empty_text"; then
  pass "no-active-session text contains '(no active session)'"
else
  fail "no-active-session text missing '(no active session)' marker"
  sed -n '1,40p' "$empty_text"
fi

set +e
empty_json="$TMPDIR/empty-json.log"
bash "$INSPECTOR" --repo-root "$empty_repo" --format json >"$empty_json" 2>&1
empty_json_rc=$?
set -e

if [[ "$empty_json_rc" -eq 0 ]]; then
  pass "no-active-session json format exits 0"
else
  fail "no-active-session json expected exit 0, got $empty_json_rc"
fi

empty_found="$(jq -r '.sessionFound' "$empty_json" 2>/dev/null || echo unknown)"
if [[ "$empty_found" == "false" ]]; then
  pass "no-active-session json sessionFound=false"
else
  fail "no-active-session json sessionFound expected false, got '$empty_found'"
fi

# --- Test 5: usage error path --------------------------------------------

set +e
usage_log="$TMPDIR/usage.log"
bash "$INSPECTOR" --last not-a-number >"$usage_log" 2>&1
usage_rc=$?
set -e

if [[ "$usage_rc" -eq 2 ]]; then
  pass "invalid --last value exits 2"
else
  fail "invalid --last expected exit 2, got $usage_rc"
fi

# --- Test 6: --latency bridge appends validation latency report ----------

set +e
latency_log="$TMPDIR/latency.log"
bash "$INSPECTOR" --repo-root "$repo" --latency --since 365 >"$latency_log" 2>&1
latency_rc=$?
set -e

if [[ "$latency_rc" -eq 0 ]]; then
  pass "--latency bridge exits 0"
else
  fail "--latency bridge expected exit 0, got $latency_rc"
  sed -n '1,120p' "$latency_log"
fi

for token in \
  "6. Validation Latency (--latency)" \
  "# Validation Latency Report" \
  "| Phase |" \
  "| implement | all | specs/777-fake | 1 | 15m0s | 15m0s | 15m0s | 30m0s | yes |"
do
  if grep -Fq "$token" "$latency_log"; then
    pass "--latency output contains '$token'"
  else
    fail "--latency output missing token: '$token'"
    sed -n '1,160p' "$latency_log"
  fi
done

# --- Test 7: --health with JSON input -----------------------------------

repo_health_input="$TMPDIR/repo-health-input"
mkdir -p "$repo_health_input/.specify/memory"
cat > "$repo_health_input/.specify/memory/bubbles.session.json" <<'EOF'
{
  "sessionId": "selftest-health-input-session",
  "turnSnapshots": [
    {
      "specDir": "specs/777-fake",
      "turnNumber": 1,
      "startedAt": "2026-05-09T11:00:00Z",
      "completedAt": "2026-05-09T11:05:00Z",
      "content": "implementation"
    },
    {
      "specDir": "specs/777-fake",
      "turnNumber": 2,
      "startedAt": "2026-05-09T11:06:00Z",
      "completedAt": "2026-05-09T11:10:00Z",
      "content": "validation"
    }
  ],
  "envelopesReceived": [
    {
      "specDir": "specs/777-fake",
      "rawSizeBytes": 1200,
      "compactedAt": "2026-05-09T11:45:00Z"
    }
  ]
}
EOF

health_json="$TMPDIR/convergence-health.json"
set +e
retro_json_log="$TMPDIR/retro-json.log"
bash "$RETRO_HEALTH" specs/777-fake --repo-root "$repo_health_input" --format json >"$health_json" 2>"$retro_json_log"
retro_json_rc=$?
set -e

if [[ "$retro_json_rc" -eq 0 ]]; then
  pass "retro-convergence-health produces JSON for --health input"
else
  fail "retro-convergence-health JSON expected exit 0, got $retro_json_rc"
  sed -n '1,80p' "$retro_json_log"
fi

set +e
health_input_log="$TMPDIR/health-input.log"
bash "$INSPECTOR" --health --input "$health_json" >"$health_input_log" 2>&1
health_input_rc=$?
set -e

if [[ "$health_input_rc" -eq 0 ]]; then
  pass "--health --input exits 0"
else
  fail "--health --input expected exit 0, got $health_input_rc"
  sed -n '1,80p' "$health_input_log"
fi

for token in \
  "Convergence Health:" \
  "turnCount=2" \
  "compactionInvocations=0" \
  "recapInvocations=0" \
  "handoffInvocations=0" \
  "blockedFindings=0" \
  "status=HEALTHY"
do
  if grep -Fq "$token" "$health_input_log"; then
    pass "--health --input output contains '$token'"
  else
    fail "--health --input output missing token: '$token'"
    sed -n '1,80p' "$health_input_log"
  fi
done

# --- Test 8: --health re-derives from session/spec ----------------------

set +e
health_spec_log="$TMPDIR/health-spec.log"
bash "$INSPECTOR" --repo-root "$repo" --health --spec specs/777-fake >"$health_spec_log" 2>&1
health_spec_rc=$?
set -e

if [[ "$health_spec_rc" -eq 0 ]]; then
  pass "--health --spec exits 0"
else
  fail "--health --spec expected exit 0, got $health_spec_rc"
  sed -n '1,80p' "$health_spec_log"
fi

for token in \
  "Convergence Health:" \
  "turnCount=2" \
  "compactionInvocations=1" \
  "recapInvocations=1" \
  "handoffInvocations=1" \
  "blockedFindings=0" \
  "status=DEGRADED"
do
  if grep -Fq "$token" "$health_spec_log"; then
    pass "--health --spec output contains '$token'"
  else
    fail "--health --spec output missing token: '$token'"
    sed -n '1,80p' "$health_spec_log"
  fi
done

# --- Summary --------------------------------------------------------------

echo
if [[ "$failures" -eq 0 ]]; then
  echo "[selftest trajectory-inspector] OK — all assertions passed"
  exit 0
else
  echo "[selftest trajectory-inspector] FAIL — $failures assertion(s) failed"
  exit 1
fi
