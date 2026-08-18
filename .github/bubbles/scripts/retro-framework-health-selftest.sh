#!/usr/bin/env bash
# retro-framework-health-selftest.sh — hermetic selftest.
#
# Cases:
#   1. No input files → proposal written with "no signal" messages
#   2. Events file with gate failures → top gates appear in proposal
#   3. Runs file with non-completed modes → stalled modes appear
#   4. Script makes ZERO writes to bubbles/, agents/, or any non-improvements path

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/retro-framework-health.sh"

[[ -x "$SCRIPT" ]] || { echo "FAIL: $SCRIPT not executable" >&2; exit 1; }

TMP="$(mktemp -d "${HOME}/.bubbles-selftest-retro-fh.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

# 1. No input files — should still write a proposal
mkdir -p "$TMP/improvements"
out_file="$("$SCRIPT" "$TMP" --slug "no-signal-test" 2>/dev/null | sed 's/.*Wrote //')"
[[ -f "$out_file" ]] || { echo "FAIL: case 1 proposal file not written ($out_file)"; exit 1; }
grep -q "no gate prevented a transition" "$out_file" || { echo "FAIL: case 1 expected the empty-gate-store marker"; exit 1; }
grep -q "no unsuccessful or pending run data" "$out_file" || { echo "FAIL: case 1 expected 'no unsuccessful or pending run data' marker"; exit 1; }
echo "PASS: no input files → proposal with no-signal messages"

# 2. Gate outcomes come from gate-hits.jsonl, the store that carries them.
#
# The RED shape this repairs: gate outcomes present ONLY in gate-hits.jsonl,
# with framework-events.jsonl holding no gate records at all. The old reader
# ranked failures out of framework-events.jsonl and reported "no gate failure
# data" against a populated store. Case 2b below proves the reader does not fall
# back to the wrong store, so this cannot silently regress.
mkdir -p "$TMP/.specify/runtime"
cat > "$TMP/.specify/runtime/framework-events.jsonl" <<'EOF'
{"kind":"agent-invocation","agent":"bubbles.plan"}
{"kind":"agent-invocation","agent":"bubbles.implement"}
EOF
cat > "$TMP/.specify/runtime/gate-hits.jsonl" <<'EOF'
{"kind":"gate","gate":"G123","outcome":"fail","fired":true,"prevented":true,"guardVerdict":"BLOCKED","exitStatus":"2"}
{"kind":"gate","gate":"G123","outcome":"fail","fired":true,"prevented":true,"guardVerdict":"BLOCKED","exitStatus":"2"}
{"kind":"gate","gate":"G124","outcome":"fail","fired":true,"prevented":false,"guardVerdict":"PASS","exitStatus":"0"}
{"kind":"gate","gate":"G099","outcome":"pass","fired":true,"prevented":false,"guardVerdict":"PASS","exitStatus":"0"}
{"kind":"gate","gate":"G055","outcome":"not-evaluated","fired":false,"prevented":false,"guardVerdict":"PASS","exitStatus":"0"}
EOF
out_file="$("$SCRIPT" "$TMP" --slug "gate-outcome-test" 2>/dev/null | sed 's/.*Wrote //')"
grep -q "G123 (2 prevented transition(s))" "$out_file" || { echo "FAIL: case 2 expected 'G123 (2 prevented transition(s))'"; cat "$out_file"; exit 1; }
grep -q "G124 (fired 1 time(s), prevented nothing)" "$out_file" || { echo "FAIL: case 2 expected G124 as fired-but-never-prevented"; cat "$out_file"; exit 1; }
grep -q "G055 (credited 1 time(s) without being evaluated)" "$out_file" || { echo "FAIL: case 2 expected G055 as credited-without-evaluation"; cat "$out_file"; exit 1; }
echo "PASS: gate outcomes ranked from gate-hits.jsonl, with prevention separated from firing"

# 2b. ADVERSARIAL: gate ranking must change when ONLY gate-hits.jsonl changes.
#     If the reader ever points back at framework-events.jsonl this fails.
cat > "$TMP/.specify/runtime/gate-hits.jsonl" <<'EOF'
{"kind":"gate","gate":"G077","outcome":"fail","fired":true,"prevented":true,"guardVerdict":"BLOCKED","exitStatus":"2"}
EOF
out_file="$("$SCRIPT" "$TMP" --slug "gate-store-sensitivity-test" 2>/dev/null | sed 's/.*Wrote //')"
grep -q "G077 (1 prevented transition(s))" "$out_file" || { echo "FAIL: case 2b ranking did not follow gate-hits.jsonl"; cat "$out_file"; exit 1; }
grep -q "G123" "$out_file" && { echo "FAIL: case 2b reported a gate absent from the current store"; cat "$out_file"; exit 1; }
echo "PASS: gate ranking changes when only gate-hits.jsonl changes"

# 2c. ADVERSARIAL: a legacy record predating fired/prevented is still counted,
#     derived from its own fields rather than dropped.
cat > "$TMP/.specify/runtime/gate-hits.jsonl" <<'EOF'
{"kind":"gate","gate":"G088","outcome":"fail","guardVerdict":"BLOCKED","exitStatus":"1"}
{"kind":"gate","gate":"G089","outcome":"fail","guardVerdict":"PASS","exitStatus":"0"}
EOF
out_file="$("$SCRIPT" "$TMP" --slug "legacy-record-test" 2>/dev/null | sed 's/.*Wrote //')"
grep -q "G088 (1 prevented transition(s))" "$out_file" || { echo "FAIL: case 2c legacy blocking record not derived as prevention"; cat "$out_file"; exit 1; }
grep -q "G089 (fired 1 time(s), prevented nothing)" "$out_file" || { echo "FAIL: case 2c legacy non-blocking record derived as prevention"; cat "$out_file"; exit 1; }
echo "PASS: legacy gate records are derived, not dropped and not over-counted"

# 2d. Capability freshness uses fields the ledger schema actually defines.
#     The old reader filtered on `lastValidated`, which no ledger entry carries,
#     so it always concluded "no stale capabilities detected".
mkdir -p "$TMP/bubbles"
cat > "$TMP/bubbles/capability-ledger.yaml" <<'EOF'
version: 1
capabilities:
  shipped-thing:
    label: Shipped thing
    state: shipped
    releaseIntroduced: v7.20.0
  partial-thing:
    label: Partial thing
    state: partial
    releaseIntroduced: v7.27.0
EOF
if command -v yq >/dev/null 2>&1; then
  out_file="$("$SCRIPT" "$TMP" --slug "capability-fields-test" 2>/dev/null | sed 's/.*Wrote //')"
  grep -q "shipped: 1 capability(ies)" "$out_file" || { echo "FAIL: case 2d expected a state distribution from the real schema"; cat "$out_file"; exit 1; }
  grep -q "partial-thing (state: partial, introduced: v7.27.0)" "$out_file" || { echo "FAIL: case 2d expected the non-shipped capability with fields that exist"; cat "$out_file"; exit 1; }
  grep -q "Capability validation age: UNMEASURED" "$out_file" || { echo "FAIL: case 2d must declare validation age unmeasured, not report a freshness figure"; cat "$out_file"; exit 1; }
  grep -q "lastValidated" "$out_file" && { echo "FAIL: case 2d reported an absent ledger field"; cat "$out_file"; exit 1; }
  echo "PASS: capability freshness reads real fields and declares validation age unmeasured"
else
  echo "SKIP: yq not installed, capability field reading"
fi

# 2e. Scenario progress reports STATE counts and never gate outcomes.
mkdir -p "$TMP/specs/001-example"
cat > "$TMP/specs/001-example/scenario-manifest.json" <<'EOF'
{"version":1,"scenarios":[{"id":"SCN-001","state":"locked"},{"id":"SCN-002","state":"locked"},{"id":"SCN-003","state":"unlocked"}]}
EOF
out_file="$("$SCRIPT" "$TMP" --slug "scenario-state-test" 2>/dev/null | sed 's/.*Wrote //')"
grep -q "locked: 2 scenario(s)" "$out_file" || { echo "FAIL: case 2e expected scenario state counts"; cat "$out_file"; exit 1; }
grep -q "unlocked: 1 scenario(s)" "$out_file" || { echo "FAIL: case 2e expected scenario state counts"; cat "$out_file"; exit 1; }
grep -q "Gate outcomes are NOT progress" "$out_file" || { echo "FAIL: case 2e must state that gate outcomes are not progress"; cat "$out_file"; exit 1; }
echo "PASS: scenario progress reports state counts, never gate-pass counts"

# 2f. Every reported signal names its producer and its store.
grep -q '`.specify/runtime/gate-hits.jsonl` | `bubbles/scripts/gate-hit-log.sh`' "$out_file" || { echo "FAIL: case 2f gate signal does not name its producer"; cat "$out_file"; exit 1; }
grep -q 'Store: `bubbles/capability-ledger.yaml`' "$out_file" || { echo "FAIL: case 2f capability section does not name its store"; cat "$out_file"; exit 1; }
echo "PASS: every reported signal names its store and producer"

# 3. Runs file with unsuccessful and still-pending runs.
#
# The previous fixture here was a top-level array of {mode, outcome} records --
# a schema workflow-runs.json has never used. It is an object holding
# activeRuns/recentRuns whose records carry command/result. Because the fixture
# agreed with the reader's wrong assumption, both were wrong together and the
# selftest passed while the real report showed "no data" against a file holding
# 22 active and 12 failed runs. The fixture now mirrors the real file.
cat > "$TMP/.specify/runtime/workflow-runs.json" <<'EOF'
{
  "version": 1,
  "activeRuns": [
    {"command":"full-delivery","status":"running"}
  ],
  "recentRuns": [
    {"command":"full-delivery","result":"success"},
    {"command":"full-delivery","result":"failed"},
    {"command":"incident-fastlane","result":"timeout"}
  ]
}
EOF
if command -v jq >/dev/null 2>&1; then
  out_file="$("$SCRIPT" "$TMP" --slug "stalled-modes-test" 2>/dev/null | sed 's/.*Wrote //')"
  # full-delivery: one active run with no result yet, plus one failed. The
  # successful one must not be counted.
  grep -q "full-delivery (2 run(s) not completed successfully)" "$out_file" || { echo "FAIL: case 3 expected 'full-delivery (2 run(s) not completed successfully)'"; cat "$out_file"; exit 1; }
  grep -q "incident-fastlane (1 run(s) not completed successfully)" "$out_file" || { echo "FAIL: case 3 expected 'incident-fastlane (1 run(s) not completed successfully)'"; cat "$out_file"; exit 1; }
  echo "PASS: stalled modes counted"
fi

# 4. No writes outside improvements/
#    Capture mtime snapshot of bubbles/ if it exists in TMP
mkdir -p "$TMP/bubbles" "$TMP/agents"
touch "$TMP/bubbles/sentinel" "$TMP/agents/sentinel"
before_b="$(stat -c %Y "$TMP/bubbles/sentinel" 2>/dev/null || stat -f %m "$TMP/bubbles/sentinel" 2>/dev/null)"
before_a="$(stat -c %Y "$TMP/agents/sentinel" 2>/dev/null || stat -f %m "$TMP/agents/sentinel" 2>/dev/null)"
"$SCRIPT" "$TMP" --slug "no-write-test" >/dev/null 2>&1
after_b="$(stat -c %Y "$TMP/bubbles/sentinel" 2>/dev/null || stat -f %m "$TMP/bubbles/sentinel" 2>/dev/null)"
after_a="$(stat -c %Y "$TMP/agents/sentinel" 2>/dev/null || stat -f %m "$TMP/agents/sentinel" 2>/dev/null)"
[[ "$before_b" == "$after_b" ]] || { echo "FAIL: case 4 bubbles/ sentinel was modified"; exit 1; }
[[ "$before_a" == "$after_a" ]] || { echo "FAIL: case 4 agents/ sentinel was modified"; exit 1; }
echo "PASS: script makes zero writes outside improvements/"

echo "All retro-framework-health selftests passed."
