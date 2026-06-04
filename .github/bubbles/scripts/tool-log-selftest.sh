#!/usr/bin/env bash
#
# bubbles tool-log-selftest.sh — selftest for tool-log.sh (v5.1 / M1).
#
# Asserts:
#   1. Log file is created at the expected path.
#   2. One JSONL line per command, valid JSON, matching schema.
#   3. Exit code is preserved (success and failure cases).
#   4. stdout/stderr stream to caller and are hashed.
#   5. SessionId and tags are recorded faithfully.
#   6. Two commands in the same session share sessionId; different sessions differ.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOOL_LOG="$SCRIPT_DIR/tool-log.sh"
SCHEMA="$SCRIPT_DIR/../schemas/tool-call.schema.json"

if [[ ! -x "$TOOL_LOG" ]]; then
  echo "tool-log-selftest: ERROR tool-log.sh not executable at $TOOL_LOG" >&2
  exit 2
fi

TEST_ROOT="$(mktemp -d -t bubbles-tool-log-selftest.XXXXXXXX)"
trap 'rm -rf "$TEST_ROOT"' EXIT
LOG_FILE="$TEST_ROOT/calls.jsonl"

failures=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; failures=$((failures + 1)); }

# Case 1: simple success command.
BUBBLES_TOOL_LOG_FILE="$LOG_FILE" \
BUBBLES_SESSION_ID="sess-A" \
BUBBLES_AGENT_NAME="bubbles.test" \
BUBBLES_SPEC="042-foo" \
BUBBLES_SCOPE="01-bar" \
BUBBLES_TOOL_LOG_TAGS="unit,fast" \
BUBBLES_TOOL_LOG_QUIET=1 \
  bash "$TOOL_LOG" /bin/echo "hello world" > "$TEST_ROOT/case1.out" 2> "$TEST_ROOT/case1.err"
case1_exit=$?

[[ "$case1_exit" -eq 0 ]] \
  && pass "Case 1: success command preserves exit 0" \
  || fail "Case 1: success command preserves exit 0 (got $case1_exit)"

grep -Fq "hello world" "$TEST_ROOT/case1.out" \
  && pass "Case 1: stdout streamed to caller" \
  || fail "Case 1: stdout streamed to caller"

[[ -f "$LOG_FILE" ]] \
  && pass "Case 1: log file created" \
  || fail "Case 1: log file created"

[[ "$(wc -l < "$LOG_FILE")" -eq 1 ]] \
  && pass "Case 1: log has exactly one line" \
  || fail "Case 1: log has exactly one line ($(wc -l < "$LOG_FILE"))"

# Validate JSON shape.
python3 -c "
import json, sys
line = open('$LOG_FILE').readlines()[0]
d = json.loads(line)
assert d['sessionId'] == 'sess-A', f'sessionId={d[\"sessionId\"]}'
assert d['agent'] == 'bubbles.test'
assert d['spec'] == '042-foo'
assert d['scope'] == '01-bar'
assert d['exitCode'] == 0
assert 'echo' in d['cmd']
assert d['tags'] == ['unit', 'fast'], f'tags={d[\"tags\"]}'
assert len(d['stdoutHash']) == 64, f'stdoutHash len={len(d[\"stdoutHash\"])}'
print('JSON_OK')
" 2>&1 | grep -Fq "JSON_OK" \
  && pass "Case 1: JSON record has all required fields with correct values" \
  || fail "Case 1: JSON record has all required fields with correct values"

# Case 2: failing command preserves exit code.
BUBBLES_TOOL_LOG_FILE="$LOG_FILE" \
BUBBLES_SESSION_ID="sess-A" \
BUBBLES_TOOL_LOG_QUIET=1 \
  bash "$TOOL_LOG" bash -c "exit 42" > /dev/null 2>&1 || case2_exit=$?

[[ "${case2_exit:-0}" -eq 42 ]] \
  && pass "Case 2: failure command preserves exit 42" \
  || fail "Case 2: failure command preserves exit 42 (got ${case2_exit:-0})"

# Verify second line in log.
[[ "$(wc -l < "$LOG_FILE")" -eq 2 ]] \
  && pass "Case 2: log appended (now 2 lines)" \
  || fail "Case 2: log appended (now 2 lines)"

python3 -c "
import json
lines = open('$LOG_FILE').readlines()
d1 = json.loads(lines[0])
d2 = json.loads(lines[1])
assert d1['sessionId'] == d2['sessionId'] == 'sess-A', 'same session must share id'
assert d2['exitCode'] == 42, f'expected 42 got {d2[\"exitCode\"]}'
print('SESSION_OK')
" 2>&1 | grep -Fq "SESSION_OK" \
  && pass "Case 2: same sessionId across calls; exit recorded" \
  || fail "Case 2: same sessionId across calls; exit recorded"

# Case 3: different sessionId.
BUBBLES_TOOL_LOG_FILE="$LOG_FILE" \
BUBBLES_SESSION_ID="sess-B" \
BUBBLES_TOOL_LOG_QUIET=1 \
  bash "$TOOL_LOG" /bin/true > /dev/null 2>&1

python3 -c "
import json
lines = open('$LOG_FILE').readlines()
ids = {json.loads(l)['sessionId'] for l in lines}
assert ids == {'sess-A', 'sess-B'}, f'expected two sessions, got {ids}'
print('MULTI_SESSION_OK')
" 2>&1 | grep -Fq "MULTI_SESSION_OK" \
  && pass "Case 3: distinct sessionId produces distinct log entries" \
  || fail "Case 3: distinct sessionId produces distinct log entries"

# Case 4: schema validation (when jsonschema is available).
if python3 -c "import yaml, jsonschema" >/dev/null 2>&1 && [[ -f "$SCHEMA" ]]; then
  python3 -c "
import json
from jsonschema import Draft7Validator
schema = json.load(open('$SCHEMA'))
validator = Draft7Validator(schema)
for i, line in enumerate(open('$LOG_FILE')):
    d = json.loads(line)
    errs = list(validator.iter_errors(d))
    assert not errs, f'line {i}: {errs[0].message}'
print('SCHEMA_OK')
" 2>&1 | grep -Fq "SCHEMA_OK" \
    && pass "Case 4: every record validates against tool-call.schema.json" \
    || fail "Case 4: every record validates against tool-call.schema.json"
else
  echo "SKIP: Case 4 schema validation (jsonschema not available)"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "tool-log-selftest: FAIL ($failures issue(s))"
  exit 1
fi

echo "tool-log-selftest: PASS"
exit 0
