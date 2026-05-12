#!/usr/bin/env bash
set -euo pipefail

# state-snapshot-selftest.sh — verify state-snapshot.sh behavior.
#
# Cases:
#   1. Append to fresh session JSON → turnNumber=1, fields present.
#   2. Append to existing session JSON → turnNumber increments,
#      prior records preserved.
#   3. --mode end after --mode start → two records present with
#      matching scopeId and a turn-start/turn-end pair.
#   4. Missing required --phase flag → exit non-zero with error message.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SNAPSHOT="$SCRIPT_DIR/state-snapshot.sh"

if [[ ! -x "$SNAPSHOT" ]]; then
  echo "FAIL: state-snapshot.sh is not executable at $SNAPSHOT" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "FAIL: jq is required for state-snapshot-selftest.sh but not found in PATH" >&2
  exit 1
fi

TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

failures=0

pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; failures=$((failures + 1)); }

echo "Running state-snapshot selftest..."
echo "Scenario: orchestrator agents must record a per-turn snapshot in .specify/memory/bubbles.session.json without ever losing prior records."

# ---- Case 1: append to fresh session JSON ---------------------------------

case1_root="$TMP_ROOT/case1"
mkdir -p "$case1_root/.specify/memory"
# Note: empty/missing session file should be created by the snapshot script.

case1_out="$(BUBBLES_REPO_ROOT="$case1_root" BUBBLES_AGENT_NAME="bubbles.workflow" \
  bash "$SNAPSHOT" --phase phase_1_plan --mode start --note "starting batch 2A" 2>&1)"

case1_session="$case1_root/.specify/memory/bubbles.session.json"

if [[ -f "$case1_session" ]]; then
  pass "Snapshot creates session JSON file when missing"
else
  fail "Snapshot must create session JSON file when missing"
  echo "  output: $case1_out"
fi

case1_count="$(jq '.turnSnapshots | length' "$case1_session" 2>/dev/null || echo -1)"
if [[ "$case1_count" == "1" ]]; then
  pass "First snapshot creates exactly one turnSnapshots entry"
else
  fail "First snapshot should create exactly one turnSnapshots entry (got $case1_count)"
fi

case1_turn="$(jq '.turnSnapshots[0].turnNumber' "$case1_session" 2>/dev/null || echo -1)"
if [[ "$case1_turn" == "1" ]]; then
  pass "First snapshot turnNumber is 1"
else
  fail "First snapshot turnNumber should be 1 (got $case1_turn)"
fi

case1_fields_ok=true
for field in phase mode agent note timestamp; do
  v="$(jq -r ".turnSnapshots[0].$field" "$case1_session" 2>/dev/null || echo "")"
  if [[ -z "$v" || "$v" == "null" ]]; then
    case1_fields_ok=false
    fail "Required field '$field' missing or null in first snapshot record"
  fi
done
if $case1_fields_ok; then
  pass "All required fields (phase, mode, agent, note, timestamp) present in first snapshot"
fi

case1_phase="$(jq -r '.turnSnapshots[0].phase' "$case1_session")"
case1_mode="$(jq -r '.turnSnapshots[0].mode' "$case1_session")"
case1_agent="$(jq -r '.turnSnapshots[0].agent' "$case1_session")"
case1_scope="$(jq -r '.turnSnapshots[0].scopeId' "$case1_session")"
if [[ "$case1_phase" == "phase_1_plan" \
   && "$case1_mode" == "start" \
   && "$case1_agent" == "bubbles.workflow" \
   && "$case1_scope" == "null" ]]; then
  pass "First snapshot record carries the supplied phase/mode/agent and null scopeId"
else
  fail "First snapshot record fields did not match supplied values"
  echo "  phase=$case1_phase mode=$case1_mode agent=$case1_agent scopeId=$case1_scope"
fi

# ---- Case 2: append to existing session JSON ------------------------------

case2_root="$TMP_ROOT/case2"
mkdir -p "$case2_root/.specify/memory"
case2_session="$case2_root/.specify/memory/bubbles.session.json"

# Seed an existing session JSON with a non-snapshot field that MUST be
# preserved across the append, plus a pre-existing turnSnapshots entry.
cat > "$case2_session" <<'JSON'
{
  "sessionId": "session-existing",
  "turnSnapshots": [
    {
      "turnNumber": 1,
      "timestamp": "2026-01-01T00:00:00Z",
      "phase": "prior_phase",
      "scopeId": "scope-prior",
      "mode": "start",
      "note": "pre-existing record",
      "agent": "bubbles.goal"
    }
  ]
}
JSON

BUBBLES_REPO_ROOT="$case2_root" BUBBLES_AGENT_NAME="bubbles.sprint" \
  bash "$SNAPSHOT" --phase phase_2_plan --scope-id scope-A --mode start >/dev/null

case2_count="$(jq '.turnSnapshots | length' "$case2_session")"
if [[ "$case2_count" == "2" ]]; then
  pass "Append to existing session yields exactly 2 turnSnapshots entries"
else
  fail "Append to existing session should yield 2 entries (got $case2_count)"
fi

case2_turn_new="$(jq '.turnSnapshots[1].turnNumber' "$case2_session")"
if [[ "$case2_turn_new" == "2" ]]; then
  pass "Appended record turnNumber correctly increments to 2"
else
  fail "Appended record turnNumber should be 2 (got $case2_turn_new)"
fi

case2_prior_phase="$(jq -r '.turnSnapshots[0].phase' "$case2_session")"
case2_prior_note="$(jq -r '.turnSnapshots[0].note' "$case2_session")"
if [[ "$case2_prior_phase" == "prior_phase" && "$case2_prior_note" == "pre-existing record" ]]; then
  pass "Pre-existing turnSnapshots[0] record is preserved verbatim"
else
  fail "Pre-existing turnSnapshots[0] record was modified"
  echo "  phase=$case2_prior_phase note=$case2_prior_note"
fi

case2_session_id="$(jq -r '.sessionId' "$case2_session")"
if [[ "$case2_session_id" == "session-existing" ]]; then
  pass "Non-snapshot session fields (sessionId) preserved across append"
else
  fail "Non-snapshot session fields should be preserved (got sessionId=$case2_session_id)"
fi

# ---- Case 3: --mode end after --mode start --------------------------------

case3_root="$TMP_ROOT/case3"
mkdir -p "$case3_root/.specify/memory"
case3_session="$case3_root/.specify/memory/bubbles.session.json"

BUBBLES_REPO_ROOT="$case3_root" BUBBLES_AGENT_NAME="bubbles.iterate" \
  bash "$SNAPSHOT" --phase phase_3_execute --scope-id scope-X --mode start >/dev/null
BUBBLES_REPO_ROOT="$case3_root" BUBBLES_AGENT_NAME="bubbles.iterate" \
  bash "$SNAPSHOT" --phase phase_3_execute --scope-id scope-X --mode end >/dev/null

case3_count="$(jq '.turnSnapshots | length' "$case3_session")"
if [[ "$case3_count" == "2" ]]; then
  pass "start+end snapshot pair produces exactly 2 records"
else
  fail "start+end pair should produce 2 records (got $case3_count)"
fi

case3_mode_a="$(jq -r '.turnSnapshots[0].mode' "$case3_session")"
case3_mode_b="$(jq -r '.turnSnapshots[1].mode' "$case3_session")"
case3_scope_a="$(jq -r '.turnSnapshots[0].scopeId' "$case3_session")"
case3_scope_b="$(jq -r '.turnSnapshots[1].scopeId' "$case3_session")"

if [[ "$case3_mode_a" == "start" && "$case3_mode_b" == "end" ]]; then
  pass "start+end pair records modes in the correct order"
else
  fail "start+end pair should record mode start then end (got $case3_mode_a, $case3_mode_b)"
fi

if [[ "$case3_scope_a" == "scope-X" && "$case3_scope_b" == "scope-X" ]]; then
  pass "start+end pair preserves matching scopeId across both records"
else
  fail "start+end pair scopeId mismatch (start=$case3_scope_a, end=$case3_scope_b)"
fi

case3_turn_a="$(jq '.turnSnapshots[0].turnNumber' "$case3_session")"
case3_turn_b="$(jq '.turnSnapshots[1].turnNumber' "$case3_session")"
if [[ "$case3_turn_a" == "1" && "$case3_turn_b" == "2" ]]; then
  pass "start+end pair turnNumbers increment correctly (1 → 2)"
else
  fail "start+end pair turnNumbers should be 1 and 2 (got $case3_turn_a, $case3_turn_b)"
fi

# ---- Case 4: missing required --phase flag --------------------------------

case4_root="$TMP_ROOT/case4"
mkdir -p "$case4_root/.specify/memory"
set +e
case4_out="$(BUBBLES_REPO_ROOT="$case4_root" \
  bash "$SNAPSHOT" --mode start 2>&1)"
case4_exit=$?
set -e

if [[ "$case4_exit" -ne 0 ]]; then
  pass "Missing --phase flag exits non-zero (exit=$case4_exit)"
else
  fail "Missing --phase flag should exit non-zero (got exit=$case4_exit)"
  echo "  output: $case4_out"
fi

if printf '%s' "$case4_out" | grep -q -- '--phase is required'; then
  pass "Missing --phase flag prints a clear error message on stderr"
else
  fail "Missing --phase flag should print a clear error message"
  echo "  output: $case4_out"
fi

# ---- Sanity: --help exits 0 ------------------------------------------------

if "$SNAPSHOT" --help >/dev/null 2>&1; then
  pass "--help exits 0"
else
  fail "--help should exit 0"
fi

if "$SNAPSHOT" --help 2>/dev/null | grep -q '^Usage:'; then
  pass "--help prints a Usage banner"
else
  fail "--help should print a Usage banner"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "state-snapshot selftest failed with $failures issue(s)."
  exit 1
fi
echo "state-snapshot selftest passed."
