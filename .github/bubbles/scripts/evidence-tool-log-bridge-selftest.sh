#!/usr/bin/env bash
#
# bubbles evidence-tool-log-bridge-selftest.sh — selftest for the bridge
# (v6.0 / B1). Validates that:
#   1. Bridge runs in text mode against a spec with no tool log.
#   2. Bridge runs in JSON mode against a spec with no tool log
#      (returns logPresent=false envelope, exit 0).
#   3. Bridge runs in text mode against a spec whose DoD items carry `Receipt:`
#      pointers answered by semantically bound tool-log entries, and reports
#      non-zero coverage.
#   4. Bridge runs in JSON mode against the same fixture and returns a
#      structured envelope with matches[] populated and coveragePct>0.
#   5. --format=json output parses as valid JSON.
#   6. Unknown --format value rejects with exit 2.
#   7. Missing spec dir argument rejects with exit 2.
#
# Replaces nothing; this is a NEW selftest registered after tool-log-selftest.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BRIDGE="$SCRIPT_DIR/evidence-tool-log-bridge.sh"

[[ -x "$BRIDGE" ]] || { echo "selftest: missing $BRIDGE" >&2; exit 2; }
command -v python3 >/dev/null 2>&1 || { echo "selftest: python3 required" >&2; exit 2; }

failures=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; failures=$((failures + 1)); }

TEST_ROOT="$(mktemp -d -t bubbles-bridge-selftest.XXXXXXXX)"
trap 'rm -rf "$TEST_ROOT"' EXIT

# Build a minimal spec dir. IMP-047 S-C: admission is SEMANTIC, so each checked
# DoD item POINTS AT the scenario whose receipt is offered as its coverage. An
# item with no pointer makes no verifiable claim and is reported as UNBOUND.
mkdir -p "$TEST_ROOT/specs/042-foo"
cat > "$TEST_ROOT/specs/042-foo/scopes.md" <<'EOF'
# Test Plan

## Scope 1

### Definition of Done

- [x] go test ./internal/foo passes — Receipt: SCN-042-001
- [x] ./bubbles.sh validate exits zero — Receipt: SCN-042-002
- [ ] still in progress
EOF

# 1+2. No tool-log file present.
out_text="$(bash "$BRIDGE" "$TEST_ROOT/specs/042-foo" 2>&1 || true)"
if grep -q 'no tool-call log' <<< "$out_text"; then
  pass "1. text mode with no log reports advisory"
else
  fail "1. text mode with no log: got '$out_text'"
fi

out_json="$(bash "$BRIDGE" "$TEST_ROOT/specs/042-foo" --format=json 2>&1 || true)"
if echo "$out_json" | python3 -c "
import json, sys
d = json.load(sys.stdin)
assert d['logPresent'] is False, d
assert d['matches'] == [], d
print('OK')
" 2>/dev/null | grep -q OK; then
  pass "2. json mode with no log returns logPresent:false envelope"
else
  fail "2. json mode with no log: got '$out_json'"
fi

# Build a tool-call log whose receipts satisfy all five bindings for both checked
# DoD items: scenario, claim, command, source revision and outcome. The claim
# must COVER its item, so it is written in the item's own words.
#
# The bridge resolves the source revision from the spec dir, which is a bare
# temp dir on a normal run (no HEAD -> empty -> any cited revision is accepted).
# Resolving it the same way here keeps the fixture correct even when TMPDIR
# happens to sit inside a git work tree.
REV="$(git -C "$TEST_ROOT/specs/042-foo" rev-parse --verify HEAD 2>/dev/null || true)"
[[ -n "$REV" ]] || REV="0000000000000000000000000000000000000001"

LOG="$TEST_ROOT/.specify/runtime/tool-calls.jsonl"
mkdir -p "$(dirname "$LOG")"
cat > "$LOG" <<EOF
{"schemaVersion":3,"sessionId":"s1","agent":"bubbles.test","spec":"042-foo","cmd":"go test ./internal/foo","exitCode":0,"ts":"2026-06-05T17:00:00Z","stdoutHash":"a","stderrHash":"b","tags":["unit"],"scenarioBinding":{"scenarioId":"SCN-042-001","phase":"green","testIdentity":"internal/foo::TestFoo","sourceRevision":"$REV","negativeControl":"revert the foo handler and TestFoo fails","claim":"go test ./internal/foo passes"}}
{"schemaVersion":3,"sessionId":"s1","agent":"bubbles.validate","spec":"042-foo","cmd":"./bubbles.sh validate run","exitCode":0,"ts":"2026-06-05T17:05:00Z","stdoutHash":"c","stderrHash":"d","tags":["validate"],"scenarioBinding":{"scenarioId":"SCN-042-002","phase":"green","testIdentity":"validate::exit-status","sourceRevision":"$REV","negativeControl":"break a validate gate and the command exits non-zero","claim":"./bubbles.sh validate exits zero"}}
EOF

# 3. text mode reports coverage > 0.
out_text2="$(bash "$BRIDGE" "$TEST_ROOT/specs/042-foo" --log "$LOG" 2>&1 || true)"
if echo "$out_text2" | grep -qE 'Coverage: [1-9][0-9]*%'; then
  pass "3. text mode with matching log reports non-zero coverage"
else
  fail "3. text mode with log: got '$out_text2'"
fi

# 4+5. JSON mode returns a non-empty matches[] envelope.
out_json2="$(bash "$BRIDGE" "$TEST_ROOT/specs/042-foo" --log "$LOG" --format=json 2>&1 || true)"
if echo "$out_json2" | python3 -c "
import json, sys
d = json.load(sys.stdin)
assert d['logPresent'] is True, d
assert d['toolLogEntries'] == 2, d
assert d['dodItems'] == 2, d
assert d['matchedDodItems'] >= 1, d
assert d['coveragePct'] >= 50, d
assert len(d['matches']) >= 1, d
m = d['matches'][0]
assert 'cmd' in m and 'ts' in m and 'scopeFile' in m and 'dodBody' in m
print('OK')
" 2>/dev/null | grep -q OK; then
  pass "4+5. json mode returns structured matches[] envelope with valid coverage stats"
else
  fail "4+5. json mode: got '$out_json2'"
fi

# 6. unknown --format value rejected.
if bash "$BRIDGE" "$TEST_ROOT/specs/042-foo" --format=xml 2>/dev/null; then
  fail "6. unknown --format value not rejected"
else
  pass "6. unknown --format value rejected with non-zero exit"
fi

# 7. missing spec dir argument rejected.
if bash "$BRIDGE" 2>/dev/null; then
  fail "7. missing spec dir not rejected"
else
  pass "7. missing spec dir rejected with non-zero exit"
fi

# 8. MCP tool spec points at --format=json (catalog wiring check).
catalog="$SCRIPT_DIR/../mcp/tools/query_tool_log.json"
if [[ -f "$catalog" ]] && grep -q -- '--format=json' "$catalog"; then
  pass "8. MCP tool catalog query_tool_log.json wires --format=json"
else
  fail "8. MCP tool catalog query_tool_log.json missing --format=json (catalog: $catalog)"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "evidence-tool-log-bridge-selftest: FAIL ($failures issue(s))"
  exit 1
fi
echo "evidence-tool-log-bridge-selftest: PASS"
exit 0
