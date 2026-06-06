#!/usr/bin/env bash
#
# parallel-fanout-determinism-selftest.sh — hermetic selftest for the v6.0 / B10
# parallel phase fan-out determinism contract (review R8, shipped v6.1).
#
# Asserts (per agents/bubbles_shared/workflow-execution-loops.md "Selftest"):
#   A. Same input DAG -> same envelope sequence across 100 runs (input order
#      shuffled each run; canonical output MUST be byte-identical).
#   B. Findings are emitted in stable (specSlug, scopeId, findingId) order.
#   C. Aggregate `at` is the LATEST individual phase timestamp.
#   D. Failure aggregation preserves ALL findings and forces route_required.
#   E. Shared-write parallel phases (forbidden by contract) are rejected at
#      dispatch time (check-dag exit 1); disjoint-write phases pass (exit 0).
#
# Exit 0 when all assertions pass; 1 otherwise.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/parallel-fanout.sh"

if ! command -v python3 >/dev/null 2>&1; then
  echo "parallel-fanout-determinism-selftest: SKIP (python3 not installed)"
  exit 0
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

pass_count=0
fail_count=0
pass() { echo "  PASS: $1"; pass_count=$((pass_count + 1)); }
fail() { echo "  FAIL: $1"; fail_count=$((fail_count + 1)); }

# --- Stage three per-phase envelopes (deliberately out of name order) --------
cat > "$TMPDIR/security.json" <<'JSON'
{ "phase": "bubbles.security", "outcome": "completed_diagnostic", "at": "2026-06-06T10:05:00Z",
  "findings": [
    { "specSlug": "066-auth", "scopeId": "SCOPE-2", "findingId": "F-002", "severity": "high", "message": "idor" },
    { "specSlug": "066-auth", "scopeId": "SCOPE-1", "findingId": "F-001", "severity": "medium", "message": "cors" }
  ] }
JSON
cat > "$TMPDIR/test.json" <<'JSON'
{ "phase": "bubbles.test", "outcome": "completed_diagnostic", "at": "2026-06-06T10:09:30Z",
  "findings": [
    { "specSlug": "066-auth", "scopeId": "SCOPE-1", "findingId": "F-003", "severity": "low", "message": "flake" }
  ] }
JSON
cat > "$TMPDIR/audit.json" <<'JSON'
{ "phase": "bubbles.audit", "outcome": "completed_diagnostic", "at": "2026-06-06T10:02:10Z",
  "findings": [] }
JSON

# --- A. 100-run shuffle invariance ------------------------------------------
baseline="$(bash "$TARGET" aggregate "$TMPDIR/security.json" "$TMPDIR/test.json" "$TMPDIR/audit.json")"
stable=1
for _ in $(seq 1 100); do
  # Shuffle the three input paths each iteration.
  mapfile -t shuffled < <(printf '%s\n' "$TMPDIR/security.json" "$TMPDIR/test.json" "$TMPDIR/audit.json" | sort -R)
  out="$(bash "$TARGET" aggregate "${shuffled[@]}")"
  if [[ "$out" != "$baseline" ]]; then
    stable=0
    echo "    DIFF on shuffled run:"
    echo "      expected: $baseline"
    echo "      actual:   $out"
    break
  fi
done
if [[ "$stable" -eq 1 ]]; then
  pass "A: 100 shuffled runs produce byte-identical canonical output"
else
  fail "A: output drifted across shuffled input order"
fi

# --- B. stable finding ordering ---------------------------------------------
# Expected finding order by (specSlug, scopeId, findingId):
#   (066-auth, SCOPE-1, F-001), (066-auth, SCOPE-1, F-003), (066-auth, SCOPE-2, F-002)
order="$(printf '%s' "$baseline" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(",".join(f["findingId"] for f in d["findings"]))')"
if [[ "$order" == "F-001,F-003,F-002" ]]; then
  pass "B: findings sorted by (specSlug, scopeId, findingId)"
else
  fail "B: finding order is '$order' (expected F-001,F-003,F-002)"
fi

# --- C. latest-at aggregation ------------------------------------------------
agg_at="$(printf '%s' "$baseline" | python3 -c 'import json,sys; print(json.load(sys.stdin)["at"])')"
if [[ "$agg_at" == "2026-06-06T10:09:30Z" ]]; then
  pass "C: aggregate at = latest phase timestamp"
else
  fail "C: aggregate at is '$agg_at' (expected 2026-06-06T10:09:30Z)"
fi

# Diagnostic-only group -> completed_diagnostic
agg_outcome="$(printf '%s' "$baseline" | python3 -c 'import json,sys; print(json.load(sys.stdin)["outcome"])')"
if [[ "$agg_outcome" == "completed_diagnostic" ]]; then
  pass "C2: all-diagnostic group -> completed_diagnostic"
else
  fail "C2: outcome is '$agg_outcome' (expected completed_diagnostic)"
fi

# --- D. failure aggregation preserves findings + forces route_required -------
cat > "$TMPDIR/failed.json" <<'JSON'
{ "phase": "bubbles.regression", "outcome": "route_required", "at": "2026-06-06T10:12:00Z",
  "findings": [
    { "specSlug": "066-auth", "scopeId": "SCOPE-3", "findingId": "F-009", "severity": "high", "message": "regression" }
  ] }
JSON
fail_agg="$(bash "$TARGET" aggregate "$TMPDIR/audit.json" "$TMPDIR/failed.json")"
fail_outcome="$(printf '%s' "$fail_agg" | python3 -c 'import json,sys; print(json.load(sys.stdin)["outcome"])')"
unresolved_n="$(printf '%s' "$fail_agg" | python3 -c 'import json,sys; print(len(json.load(sys.stdin)["unresolvedFindings"]))')"
if [[ "$fail_outcome" == "route_required" && "$unresolved_n" == "1" ]]; then
  pass "D: failed phase -> route_required with unresolvedFindings preserved"
else
  fail "D: outcome='$fail_outcome' unresolved=$unresolved_n (expected route_required / 1)"
fi

# --- E. DAG conflict detection ----------------------------------------------
# Safe: two read-only phases against the same source, disjoint writes.
cat > "$TMPDIR/dag-safe.json" <<'JSON'
{ "phases": [
  { "name": "security", "reads": ["src/"], "writes": [".cache/sec/"], "idempotent": true },
  { "name": "test",     "reads": ["src/"], "writes": [".cache/test/"], "idempotent": true }
] }
JSON
rc=0; bash "$TARGET" check-dag "$TMPDIR/dag-safe.json" >/dev/null 2>&1 || rc=$?
if [[ "$rc" -eq 0 ]]; then
  pass "E1: disjoint-write parallel group is accepted"
else
  fail "E1: safe DAG rejected (exit $rc)"
fi

# Unsafe: shared write to state.json, not idempotent.
cat > "$TMPDIR/dag-shared.json" <<'JSON'
{ "phases": [
  { "name": "implement-a", "reads": ["spec.md"], "writes": ["state.json"], "idempotent": false },
  { "name": "implement-b", "reads": ["spec.md"], "writes": ["state.json"], "idempotent": false }
] }
JSON
rc=0; bash "$TARGET" check-dag "$TMPDIR/dag-shared.json" >/dev/null 2>&1 || rc=$?
if [[ "$rc" -eq 1 ]]; then
  pass "E2: shared-write parallel group is rejected"
else
  fail "E2: shared-write DAG NOT rejected (exit $rc)"
fi

# Unsafe: data dependency (test reads what implement writes).
cat > "$TMPDIR/dag-dep.json" <<'JSON'
{ "phases": [
  { "name": "implement", "reads": ["spec.md"], "writes": ["src/api.rs"], "idempotent": false },
  { "name": "test",      "reads": ["src/api.rs"], "writes": [".cache/test/"], "idempotent": true }
] }
JSON
rc=0; bash "$TARGET" check-dag "$TMPDIR/dag-dep.json" >/dev/null 2>&1 || rc=$?
if [[ "$rc" -eq 1 ]]; then
  pass "E3: data-dependency parallel group is rejected"
else
  fail "E3: data-dependency DAG NOT rejected (exit $rc)"
fi

echo ""
echo "[parallel-fanout-determinism-selftest] $pass_count passed, $fail_count failed"
[[ "$fail_count" -eq 0 ]] || exit 1
echo "[parallel-fanout-determinism-selftest] OK"
exit 0
