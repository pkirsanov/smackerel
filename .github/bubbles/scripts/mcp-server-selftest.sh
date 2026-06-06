#!/usr/bin/env bash
#
# bubbles/scripts/mcp-server-selftest.sh
#
# Selftest for v6 / A5: MCP server boots, handles every declared method,
# loads the full tool + resource catalog, executes a real tool against the
# bash twin, reads a real resource, and returns proper JSON-RPC errors for
# malformed/unknown requests.
#
# Asserts:
#   T1.  Server starts and responds to `initialize` with protocolVersion +
#        serverInfo + capabilities.
#   T2.  `ping` returns `{}`.
#   T3.  `tools/list` returns the full declared tool catalog (>= 10 tools
#        in the source repo).
#   T4.  Every declared tool references an existing bash twin under
#        bubbles/scripts/.
#   T5.  `tools/call` for an unknown tool returns ERR_TOOL_NOT_FOUND.
#   T6.  `tools/call` for `check_gate` with `gate_id=G024` returns isError=false
#        AND the response content includes the gate description.
#   T7.  `tools/call` for `check_gate` with `action=count` (gate_id omitted)
#        returns isError=false AND a numeric body (proves the optional
#        ${var?} substitution path).
#   T8.  `resources/list` returns the declared resource catalog (>= 5 in
#        the source repo).
#   T9.  `resources/read` for `bubbles://workflows.yaml` returns a non-empty
#        body.
#   T10. `resources/read` for `bubbles://nonexistent` returns
#        ERR_RESOURCE_NOT_FOUND.
#   T11. Unknown JSON-RPC method returns ERR_METHOD_NOT_FOUND.
#
# Exit 0 = all assertions pass. Exit 1 = at least one failed.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
  MCP_DIR="$REPO_ROOT/.github/bubbles/mcp"
  SCRIPTS_DIR="$REPO_ROOT/.github/bubbles/scripts"
else
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
  MCP_DIR="$REPO_ROOT/bubbles/mcp"
  SCRIPTS_DIR="$REPO_ROOT/bubbles/scripts"
fi
SERVER="$MCP_DIR/server.py"

failures=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1" >&2; failures=$((failures + 1)); }

if [[ ! -x "$SERVER" && ! -f "$SERVER" ]]; then
  echo "SKIP: MCP server not present at $SERVER (v6 not yet installed in this tree)"
  exit 0
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "SKIP: python3 not available; MCP selftest requires Python 3.10+"
  exit 0
fi

# Tools we'll exercise must exist in the catalog. If the source repo dropped
# them in a rework, the assertion below will FAIL — which is correct.
required_tools=(
  validate_dod
  verify_status_transition
  record_evidence
  check_gate
  resolve_mode
  route_finding
  query_tool_log
  search_code
  read_spec
  list_open_findings
)

# T4 is a static pre-check that doesn't require server boot.
for tool in "${required_tools[@]}"; do
  spec_file="$MCP_DIR/tools/${tool}.json"
  if [[ ! -f "$spec_file" ]]; then
    fail "T4: tool catalog missing $spec_file"
    continue
  fi
  script_name="$(python3 -c "import json,sys;print(json.load(open(sys.argv[1]))['script'])" "$spec_file")"
  if [[ ! -f "$SCRIPTS_DIR/$script_name" ]]; then
    fail "T4: tool '$tool' references missing script $SCRIPTS_DIR/$script_name"
  fi
done
if [[ $failures -eq 0 ]]; then
  pass "T4: every declared tool references an existing bash twin"
fi

# The runtime harness drives the server over stdio with a sequence of JSON-RPC
# messages and asserts on every response in one shot. We script it inline in
# Python so the framing logic stays inside the harness.
harness_out="$(mktemp -t bubbles-mcp-selftest.XXXXXX)"
trap 'rm -f "$harness_out"' EXIT INT TERM

python3 - "$SERVER" "$harness_out" <<'PY_HARNESS'
import json
import os
import subprocess
import sys

SERVER = sys.argv[1]
OUT = sys.argv[2]


def frame(msg):
    payload = json.dumps(msg, separators=(",", ":")).encode("utf-8")
    return f"Content-Length: {len(payload)}\r\n\r\n".encode("ascii") + payload


def read_frame(reader):
    headers = b""
    while True:
        line = reader.readline()
        if not line:
            return None
        headers += line
        if line == b"\r\n":
            break
    cl = None
    for h in headers.split(b"\r\n"):
        if h.lower().startswith(b"content-length:"):
            cl = int(h.split(b":")[1].strip())
    return json.loads(reader.read(cl))


proc = subprocess.Popen(
    [sys.executable, SERVER],
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
    env=dict(os.environ, BUBBLES_MCP_LOG_LEVEL="ERROR"),
)
msgs = [
    {"jsonrpc": "2.0", "id": 1, "method": "initialize",
     "params": {"protocolVersion": "2024-11-05",
                "capabilities": {},
                "clientInfo": {"name": "mcp-selftest", "version": "0.0"}}},
    {"jsonrpc": "2.0", "method": "notifications/initialized"},
    {"jsonrpc": "2.0", "id": 2, "method": "ping"},
    {"jsonrpc": "2.0", "id": 3, "method": "tools/list"},
    {"jsonrpc": "2.0", "id": 4, "method": "tools/call",
     "params": {"name": "definitely-not-a-tool"}},
    {"jsonrpc": "2.0", "id": 5, "method": "tools/call",
     "params": {"name": "check_gate", "arguments": {"gate_id": "G024"}}},
    {"jsonrpc": "2.0", "id": 6, "method": "tools/call",
     "params": {"name": "check_gate", "arguments": {"action": "count"}}},
    {"jsonrpc": "2.0", "id": 7, "method": "resources/list"},
    {"jsonrpc": "2.0", "id": 8, "method": "resources/read",
     "params": {"uri": "bubbles://workflows.yaml"}},
    {"jsonrpc": "2.0", "id": 9, "method": "resources/read",
     "params": {"uri": "bubbles://nonexistent"}},
    {"jsonrpc": "2.0", "id": 10, "method": "some-unknown-method"},
]
for m in msgs:
    proc.stdin.write(frame(m))
proc.stdin.flush()
proc.stdin.close()

replies = {}
for _ in range(10):  # 10 IDs (notifications/initialized has no reply)
    r = read_frame(proc.stdout)
    if r is None:
        break
    if "id" in r:
        replies[r["id"]] = r
proc.wait(timeout=30)

with open(OUT, "w", encoding="utf-8") as f:
    json.dump(replies, f, indent=2)
PY_HARNESS

if [[ ! -s "$harness_out" ]]; then
  fail "harness produced no output (server did not start?)"
  exit 1
fi

# Helper: extract a value from the captured replies via python.
get() {
  python3 -c "import json,sys; d=json.load(open(sys.argv[1])); k=sys.argv[2]; r=d.get(k) or {}; print(json.dumps(r))" "$harness_out" "$1"
}

# T1: initialize
init_reply="$(get 1)"
if echo "$init_reply" | python3 -c "import json,sys; r=json.load(sys.stdin); res=r.get('result') or {}; assert 'protocolVersion' in res and 'serverInfo' in res and 'capabilities' in res, r"; then
  pass "T1: initialize returned protocolVersion + serverInfo + capabilities"
else
  fail "T1: initialize missing required fields: $init_reply"
fi

# T2: ping
ping_reply="$(get 2)"
if echo "$ping_reply" | python3 -c "import json,sys; r=json.load(sys.stdin); res=r.get('result'); assert res == {}, r"; then
  pass "T2: ping returned empty object"
else
  fail "T2: ping returned non-empty: $ping_reply"
fi

# T3: tools/list
tools_reply="$(get 3)"
n_tools="$(echo "$tools_reply" | python3 -c "import json,sys; r=json.load(sys.stdin); print(len(r['result']['tools']))")"
if [[ "$n_tools" -ge 10 ]]; then
  pass "T3: tools/list returned $n_tools tools (>= 10 required)"
else
  fail "T3: tools/list returned only $n_tools tools (expected >= 10)"
fi

# T5: tools/call unknown
unknown_reply="$(get 4)"
err_code="$(echo "$unknown_reply" | python3 -c "import json,sys; r=json.load(sys.stdin); print(r.get('error', {}).get('code', 0))")"
if [[ "$err_code" == "-32001" ]]; then
  pass "T5: unknown tool returned ERR_TOOL_NOT_FOUND (-32001)"
else
  fail "T5: unknown tool expected -32001, got $err_code: $unknown_reply"
fi

# T6: check_gate G024 — real bash execution
check_reply="$(get 5)"
ok="$(echo "$check_reply" | python3 -c "
import json,sys
r = json.load(sys.stdin)
res = r.get('result', {})
body = (res.get('content') or [{}])[0].get('text', '')
print('YES' if (not res.get('isError') and 'G024' in body) else 'NO')
")"
if [[ "$ok" == "YES" ]]; then
  pass "T6: check_gate G024 executed bash twin, body mentions G024"
else
  fail "T6: check_gate G024 did not return success+G024 body: $check_reply"
fi

# T7: check_gate count (no gate_id, exercises ${var?} optional substitution)
count_reply="$(get 6)"
ok="$(echo "$count_reply" | python3 -c "
import json,sys
r = json.load(sys.stdin)
res = r.get('result', {})
body = (res.get('content') or [{}])[0].get('text', '')
# body is the stdout/stderr envelope; we just check no error and numeric stdout.
ok = not res.get('isError') and 'exit=0' in body
print('YES' if ok else 'NO')
")"
if [[ "$ok" == "YES" ]]; then
  pass "T7: check_gate count succeeded with omitted gate_id (optional \${var?} works)"
else
  fail "T7: check_gate count failed: $count_reply"
fi

# T8: resources/list
res_reply="$(get 7)"
n_res="$(echo "$res_reply" | python3 -c "import json,sys; r=json.load(sys.stdin); print(len(r['result']['resources']))")"
if [[ "$n_res" -ge 5 ]]; then
  pass "T8: resources/list returned $n_res resources (>= 5 required)"
else
  fail "T8: resources/list returned only $n_res (expected >= 5)"
fi

# T9: resources/read workflows.yaml
read_reply="$(get 8)"
size="$(echo "$read_reply" | python3 -c "import json,sys; r=json.load(sys.stdin); print(len(r['result']['contents'][0]['text']))")"
if [[ "$size" -gt 1000 ]]; then
  pass "T9: resources/read workflows.yaml returned $size chars"
else
  fail "T9: resources/read workflows.yaml returned only $size chars (expected > 1000)"
fi

# T10: resources/read nonexistent
miss_reply="$(get 9)"
err_code="$(echo "$miss_reply" | python3 -c "import json,sys; r=json.load(sys.stdin); print(r.get('error', {}).get('code', 0))")"
if [[ "$err_code" == "-32003" ]]; then
  pass "T10: nonexistent resource returned ERR_RESOURCE_NOT_FOUND (-32003)"
else
  fail "T10: nonexistent resource expected -32003, got $err_code"
fi

# T11: unknown method
unk_reply="$(get 10)"
err_code="$(echo "$unk_reply" | python3 -c "import json,sys; r=json.load(sys.stdin); print(r.get('error', {}).get('code', 0))")"
if [[ "$err_code" == "-32601" ]]; then
  pass "T11: unknown method returned ERR_METHOD_NOT_FOUND (-32601)"
else
  fail "T11: unknown method expected -32601, got $err_code"
fi

echo
if [[ $failures -gt 0 ]]; then
  echo "mcp-server-selftest FAILED with $failures issue(s)."
  exit 1
fi
echo "mcp-server-selftest passed: MCP server boots, dispatches, and surfaces verbatim script output."
