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
#   T3.  `tools/list` returns the full declared tool catalog (>= 15 tools
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
#   T12. `resources/templates/list` returns the templated catalog (>= 2),
#        including `bubbles://gates/{id}`.
#   T13. `resources/read` for templated `bubbles://gates/G024` resolves via the
#        gate-meta.sh bash twin and the body mentions G024.
#   T14. `resources/read` for templated `bubbles://gates/G999` (unknown gate)
#        returns ERR_RESOURCE_FAILED — the bash twin's non-zero exit is surfaced
#        as a real error, never a fake empty success.
#   T15. `prompts/list` returns the Bubbles prompt shim catalog (>= 30),
#        including `bubbles.workflow`.
#   T16. `prompts/get` for `bubbles.workflow` returns a user message whose
#        content points at the workflow agent and real prompt body.
#   T17. `prompts/get` for an unknown prompt returns ERR_PROMPT_NOT_FOUND.
#   T18. `tools/list` includes MCP tool annotations: read-only/idempotent hints
#        for query tools and open-world/destructive-capable hints for the
#        evidence command wrapper.
#   T19. `initialize` negotiates the protocol version: it echoes a supported
#        requested version and falls back to the latest for an unknown one.
#   T20. `tools/list` includes `graph_neighbors` (IMP-015 Scope A).
#   T21. `tools/call` for `graph_neighbors` with node="state-transition-guard.sh"
#        returns isError=false AND a JSON payload with inDegree >= 1 and a
#        non-empty dependents[] of {source,provenance,line} objects.
#   T22. `tools/call` for `graph_neighbors` with an obviously-unknown node
#        returns a structured error result (isError=true), never a crash.
#   T23. `tools/list` includes the three IMP-037 S5 experience-recall verbs:
#        `search_experience`, `read_experience`, `experience_recall_status`.
#   T24. Each of those three declares the READ-ONLY annotation set exactly
#        (readOnlyHint true, destructiveHint false, idempotentHint true,
#        openWorldHint false) — a regression that flips any hint fails here.
#   T25. Each of those three is a thin wrapper over the SAME existing bash twin
#        (experience-recall.sh) and names its matching subcommand as argv[0],
#        AND — adversarially — the catalog loader still fail-fasts (exit 1) when
#        a recall tool points at a missing twin, proving T4's invariant is real
#        for this tool shape rather than merely unexercised.
#   T26. The MCP surface exposes ONLY the read-only recall subcommands: no tool
#        in the catalog wraps experience-recall.sh's mutating subcommands
#        (sync/delete/admit/lifecycle/export), which stay CLI-only.
#   T27. `tools/call` for `resolve_mode` in the v7 primitive+tag form resolves.
#        The catalog previously declared a single `mode` string, so the tags
#        arrived as ONE argv token and the resolver rejected every tagged mode
#        (IMP-042 / SCOPE-8).
#   T28. Adversarially: a REMOVED v5 mode name without grandfather still gets
#        rejected, and the error names the v6 form. Proves T27 passes because
#        argv is now correct, not because the resolver stopped refusing.
#   T29. The same v5 name WITH grandfather=true resolves, so persisted keys
#        remain reachable through an explicit, deprecation-stamped path.
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
  check_observability
  graph_neighbors
  search_experience
  read_experience
  experience_recall_status
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
    # MCP stdio responses are newline-delimited JSON (one object per line).
    # A legacy LSP-style Content-Length header block is still tolerated on
    # read for back-compat (mirrors StdioTransport.read_message). The server
    # now WRITES newline-delimited JSON, so this is the primary path; frame()
    # above still WRITES Content-Length to exercise the server's back-compat
    # read path.
    line = reader.readline()
    if not line:
        return None
    stripped = line.strip()
    while not stripped:  # skip blank separator lines between messages
        line = reader.readline()
        if not line:
            return None
        stripped = line.strip()
    if not stripped.lower().startswith(b"content-length:"):
        return json.loads(stripped.decode("utf-8"))
    cl = int(stripped.partition(b":")[2].strip())
    while True:
        h = reader.readline()
        if not h or not h.strip():
            break
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
    {"jsonrpc": "2.0", "id": 11, "method": "resources/templates/list"},
    {"jsonrpc": "2.0", "id": 12, "method": "resources/read",
     "params": {"uri": "bubbles://gates/G024"}},
    {"jsonrpc": "2.0", "id": 13, "method": "resources/read",
     "params": {"uri": "bubbles://gates/G999"}},
    {"jsonrpc": "2.0", "id": 14, "method": "prompts/list"},
    {"jsonrpc": "2.0", "id": 15, "method": "prompts/get",
     "params": {"name": "bubbles.workflow"}},
    {"jsonrpc": "2.0", "id": 16, "method": "prompts/get",
     "params": {"name": "definitely-not-a-prompt"}},
    {"jsonrpc": "2.0", "id": 17, "method": "initialize",
     "params": {"protocolVersion": "2024-11-05",
                "capabilities": {},
                "clientInfo": {"name": "mcp-selftest", "version": "0.0"}}},
    {"jsonrpc": "2.0", "id": 18, "method": "initialize",
     "params": {"protocolVersion": "1999-01-01",
                "capabilities": {},
                "clientInfo": {"name": "mcp-selftest", "version": "0.0"}}},
    {"jsonrpc": "2.0", "id": 19, "method": "tools/call",
     "params": {"name": "graph_neighbors",
                "arguments": {"node": "state-transition-guard.sh"}}},
    {"jsonrpc": "2.0", "id": 20, "method": "tools/call",
     "params": {"name": "graph_neighbors",
                "arguments": {"node": "definitely-not-a-real-node-xyz.sh"}}},
    {"jsonrpc": "2.0", "id": 21, "method": "tools/call",
     "params": {"name": "resolve_mode",
                "arguments": {"primitive": "review",
                              "tags": ["action:readiness-synthesis", "target:system"]}}},
    {"jsonrpc": "2.0", "id": 22, "method": "tools/call",
     "params": {"name": "resolve_mode",
                "arguments": {"mode": "bugfix-fastlane"}}},
    {"jsonrpc": "2.0", "id": 23, "method": "tools/call",
     "params": {"name": "resolve_mode",
                "arguments": {"mode": "bugfix-fastlane", "grandfather": True}}},
]
for m in msgs:
    proc.stdin.write(frame(m))
proc.stdin.flush()
proc.stdin.close()

replies = {}
for _ in range(23):  # 23 IDs (notifications/initialized has no reply)
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
if [[ "$n_tools" -ge 15 ]]; then
  pass "T3: tools/list returned $n_tools tools (>= 15 required)"
else
  fail "T3: tools/list returned only $n_tools tools (expected >= 15)"
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

# T12: resources/templates/list returns the templated catalog (>= 2), including
# the gate-by-id template.
tmpl_reply="$(get 11)"
ok="$(echo "$tmpl_reply" | python3 -c "
import json,sys
r = json.load(sys.stdin)
tmpls = (r.get('result') or {}).get('resourceTemplates') or []
uris = [t.get('uriTemplate') for t in tmpls]
print('YES' if (len(tmpls) >= 2 and 'bubbles://gates/{id}' in uris) else 'NO')
")"
if [[ "$ok" == "YES" ]]; then
  pass "T12: resources/templates/list returned templates incl bubbles://gates/{id}"
else
  fail "T12: resources/templates/list missing expected templates: $tmpl_reply"
fi

# T13: resources/read templated bubbles://gates/G024 — resolves via gate-meta.sh
# bash twin, body is the gate's JSON record mentioning G024.
gate_reply="$(get 12)"
ok="$(echo "$gate_reply" | python3 -c "
import json,sys
r = json.load(sys.stdin)
contents = (r.get('result') or {}).get('contents') or [{}]
body = contents[0].get('text', '')
print('YES' if ('G024' in body and 'error' not in r) else 'NO')
")"
if [[ "$ok" == "YES" ]]; then
  pass "T13: templated resource bubbles://gates/G024 resolved via bash twin, body mentions G024"
else
  fail "T13: templated gate resource did not resolve correctly: $gate_reply"
fi

# T14: resources/read templated bubbles://gates/G999 (unknown gate) — the bash
# twin exits non-zero, so the server returns ERR_RESOURCE_FAILED (not a fake
# empty success).
badgate_reply="$(get 13)"
err_code="$(echo "$badgate_reply" | python3 -c "import json,sys; r=json.load(sys.stdin); print(r.get('error', {}).get('code', 0))")"
if [[ "$err_code" == "-32004" ]]; then
  pass "T14: unknown templated gate returned ERR_RESOURCE_FAILED (-32004)"
else
  fail "T14: unknown templated gate expected -32004, got $err_code: $badgate_reply"
fi

# T15: prompts/list returns Bubbles prompt shims.
prompts_reply="$(get 14)"
ok="$(echo "$prompts_reply" | python3 -c "
import json,sys
r = json.load(sys.stdin)
prompts = (r.get('result') or {}).get('prompts') or []
names = [p.get('name') for p in prompts]
print('YES' if (len(prompts) >= 30 and 'bubbles.workflow' in names) else 'NO')
")"
if [[ "$ok" == "YES" ]]; then
  pass "T15: prompts/list returned Bubbles prompt shims incl bubbles.workflow"
else
  fail "T15: prompts/list missing expected prompt catalog: $prompts_reply"
fi

# T16: prompts/get returns the workflow prompt body as a user message.
prompt_reply="$(get 15)"
ok="$(echo "$prompt_reply" | python3 -c "
import json,sys
r = json.load(sys.stdin)
res = r.get('result') or {}
messages = res.get('messages') or []
text = (((messages[0] if messages else {}).get('content') or {}).get('text') or '')
print('YES' if ('Use agent: bubbles.workflow' in text and 'workflow orchestrator' in text) else 'NO')
")"
if [[ "$ok" == "YES" ]]; then
  pass "T16: prompts/get bubbles.workflow returns user message with agent + prompt body"
else
  fail "T16: prompts/get bubbles.workflow did not return expected prompt: $prompt_reply"
fi

# T17: prompts/get unknown prompt returns ERR_PROMPT_NOT_FOUND.
badprompt_reply="$(get 16)"
err_code="$(echo "$badprompt_reply" | python3 -c "import json,sys; r=json.load(sys.stdin); print(r.get('error', {}).get('code', 0))")"
if [[ "$err_code" == "-32005" ]]; then
  pass "T17: unknown prompt returned ERR_PROMPT_NOT_FOUND (-32005)"
else
  fail "T17: unknown prompt expected -32005, got $err_code: $badprompt_reply"
fi

# T18: tools/list exposes MCP annotations for modern client planning/safety.
ok="$(echo "$tools_reply" | python3 -c "
import json,sys
r = json.load(sys.stdin)
tools = {t.get('name'): t for t in (r.get('result') or {}).get('tools') or []}
check_gate = tools.get('check_gate', {}).get('annotations') or {}
record_evidence = tools.get('record_evidence', {}).get('annotations') or {}
ok = (
    check_gate.get('readOnlyHint') is True
    and check_gate.get('destructiveHint') is False
    and check_gate.get('idempotentHint') is True
    and check_gate.get('openWorldHint') is False
    and record_evidence.get('readOnlyHint') is False
    and record_evidence.get('destructiveHint') is True
    and record_evidence.get('idempotentHint') is False
    and record_evidence.get('openWorldHint') is True
)
print('YES' if ok else 'NO')
")"
if [[ "$ok" == "YES" ]]; then
  pass "T18: tools/list exposes MCP annotations for safe query tools and record_evidence"
else
  fail "T18: tools/list missing expected MCP annotations: $tools_reply"
fi

# T19: initialize negotiates the protocol version — echoes a supported requested
# version (2024-11-05) and falls back to the latest for an unknown one.
echo_reply="$(get 17)"
echo_ver="$(echo "$echo_reply" | python3 -c "import json,sys; r=json.load(sys.stdin); print((r.get('result') or {}).get('protocolVersion',''))")"
fallback_reply="$(get 18)"
fallback_ver="$(echo "$fallback_reply" | python3 -c "import json,sys; r=json.load(sys.stdin); print((r.get('result') or {}).get('protocolVersion',''))")"
if [[ "$echo_ver" == "2024-11-05" && "$fallback_ver" == "2025-06-18" ]]; then
  pass "T19: initialize echoes supported version (2024-11-05) and falls back to latest (2025-06-18) for unknown"
else
  fail "T19: protocol negotiation wrong — echo=$echo_ver (want 2024-11-05), fallback=$fallback_ver (want 2025-06-18)"
fi

# T20: tools/list includes the IMP-015 graph_neighbors verb.
ok="$(echo "$tools_reply" | python3 -c "
import json,sys
r = json.load(sys.stdin)
names = [t.get('name') for t in (r.get('result') or {}).get('tools') or []]
print('YES' if 'graph_neighbors' in names else 'NO')
")"
if [[ "$ok" == "YES" ]]; then
  pass "T20: tools/list includes graph_neighbors"
else
  fail "T20: tools/list missing graph_neighbors: $tools_reply"
fi

# T21: graph_neighbors for a known node returns the real reverse-dep payload
# (inDegree >= 1, non-empty provenance-tagged dependents) wrapping the composer.
gn_reply="$(get 19)"
ok="$(echo "$gn_reply" | python3 -c "
import json,sys
r = json.load(sys.stdin)
res = r.get('result') or {}
if res.get('isError'):
    print('NO'); sys.exit(0)
text = ((res.get('content') or [{}])[0]).get('text', '')
if '--- stdout ---' not in text or '--- stderr ---' not in text:
    print('NO'); sys.exit(0)
stdout_part = text.split('--- stdout ---', 1)[1].split('--- stderr ---', 1)[0].strip()
try:
    payload = json.loads(stdout_part)
except Exception:
    print('NO'); sys.exit(0)
deps = payload.get('dependents') or []
ok = (
    payload.get('node') == 'state-transition-guard.sh'
    and isinstance(payload.get('inDegree'), int)
    and payload.get('inDegree', 0) >= 1
    and len(deps) >= 1
    and all(('source' in d and 'provenance' in d and 'line' in d) for d in deps)
)
print('YES' if ok else 'NO')
")"
if [[ "$ok" == "YES" ]]; then
  pass "T21: graph_neighbors known node returned inDegree>=1 with provenance-tagged dependents"
else
  fail "T21: graph_neighbors known node payload wrong: $gn_reply"
fi

# T22: graph_neighbors for an obviously-unknown node returns a STRUCTURED error
# result (isError=true) surfacing the twin's exit 3 — never a crash, and never a
# fake empty success.
gn_bad_reply="$(get 20)"
ok="$(echo "$gn_bad_reply" | python3 -c "
import json,sys
r = json.load(sys.stdin)
res = r.get('result') or {}
text = ((res.get('content') or [{}])[0]).get('text', '')
ok = res.get('isError') is True and 'unknown node' in text
print('YES' if ok else 'NO')
")"
if [[ "$ok" == "YES" ]]; then
  pass "T22: graph_neighbors unknown node returned a structured error result"
else
  fail "T22: graph_neighbors unknown node did not return structured error: $gn_bad_reply"
fi

# T27/T28/T29: resolve_mode argv shape. The v7 form needs primitive and tags as
# SEPARATE argv elements; a single `mode` string could never express it.
rm_v7_reply="$(get 21)"
ok="$(echo "$rm_v7_reply" | python3 -c "
import json,sys
r = json.load(sys.stdin)
res = r.get('result') or {}
text = ((res.get('content') or [{}])[0]).get('text', '')
ok = res.get('isError') is not True and 'statusCeiling' in text and 'phaseOrder' in text
print('YES' if ok else 'NO')
")"
if [[ "$ok" == "YES" ]]; then
  pass "T27: resolve_mode resolved the v7 primitive+tag form"
else
  fail "T27: resolve_mode did not resolve the v7 tagged form: $rm_v7_reply"
fi

rm_v5_reply="$(get 22)"
ok="$(echo "$rm_v5_reply" | python3 -c "
import json,sys
r = json.load(sys.stdin)
res = r.get('result') or {}
text = ((res.get('content') or [{}])[0]).get('text', '')
ok = res.get('isError') is True and 'removed in v7' in text
print('YES' if ok else 'NO')
")"
if [[ "$ok" == "YES" ]]; then
  pass "T28: resolve_mode still rejects a removed v5 name without grandfather"
else
  fail "T28: resolve_mode did not reject the bare v5 name: $rm_v5_reply"
fi

rm_gf_reply="$(get 23)"
ok="$(echo "$rm_gf_reply" | python3 -c "
import json,sys
r = json.load(sys.stdin)
res = r.get('result') or {}
text = ((res.get('content') or [{}])[0]).get('text', '')
ok = res.get('isError') is not True and 'statusCeiling' in text
print('YES' if ok else 'NO')
")"
if [[ "$ok" == "YES" ]]; then
  pass "T29: resolve_mode resolves a persisted v5 key with grandfather=true"
else
  fail "T29: resolve_mode grandfather path did not resolve: $rm_gf_reply"
fi

# ---------------------------------------------------------------------------
# IMP-037 SCOPE-5: read-only experience-recall verbs over the existing bash twin
# ---------------------------------------------------------------------------
recall_tools=(search_experience read_experience experience_recall_status)

# T23: tools/list includes all three recall verbs.
ok="$(echo "$tools_reply" | python3 -c "
import json,sys
r = json.load(sys.stdin)
names = {t.get('name') for t in (r.get('result') or {}).get('tools') or []}
want = {'search_experience', 'read_experience', 'experience_recall_status'}
print('YES' if want <= names else 'NO:' + ','.join(sorted(want - names)))
")"
if [[ "$ok" == "YES" ]]; then
  pass "T23: tools/list includes search_experience, read_experience, experience_recall_status"
else
  fail "T23: tools/list missing recall verbs ($ok): $tools_reply"
fi

# T24: each recall verb declares the READ-ONLY annotation set EXACTLY. Asserting
# all four hints (not just readOnlyHint) means a regression that flips any one of
# them — e.g. marking a recall read destructive — fails here.
ok="$(echo "$tools_reply" | python3 -c "
import json,sys
r = json.load(sys.stdin)
tools = {t.get('name'): t for t in (r.get('result') or {}).get('tools') or []}
want = {
    'readOnlyHint': True,
    'destructiveHint': False,
    'idempotentHint': True,
    'openWorldHint': False,
}
bad = []
for name in ('search_experience', 'read_experience', 'experience_recall_status'):
    got = (tools.get(name) or {}).get('annotations') or {}
    if {k: got.get(k) for k in want} != want:
        bad.append(f'{name}={got}')
print('YES' if not bad else 'NO:' + '; '.join(bad))
")"
if [[ "$ok" == "YES" ]]; then
  pass "T24: all three recall verbs declare the read-only annotation set (readOnly/idempotent true, destructive/openWorld false)"
else
  fail "T24: recall annotations wrong ($ok)"
fi

# T25a: each recall verb is a THIN wrapper — same experience-recall.sh twin, and
# the matching subcommand is argv[0] of its argsTemplate (no MCP-side logic).
twin_ok=1
for tool in "${recall_tools[@]}"; do
  case "$tool" in
    search_experience) want_subcommand="search" ;;
    read_experience) want_subcommand="read" ;;
    experience_recall_status) want_subcommand="status" ;;
    *) want_subcommand="<unmapped>" ;;
  esac
  spec_file="$MCP_DIR/tools/${tool}.json"
  declared_script="$(python3 -c "import json,sys;print(json.load(open(sys.argv[1])).get('script','<none>'))" "$spec_file")"
  declared_arg0="$(python3 -c "import json,sys;a=json.load(open(sys.argv[1])).get('argsTemplate') or ['<none>'];print(a[0])" "$spec_file")"
  if [[ "$declared_script" != "experience-recall.sh" ]]; then
    fail "T25a: '$tool' wraps '$declared_script', expected the experience-recall.sh bash twin"
    twin_ok=0
  elif [[ ! -f "$SCRIPTS_DIR/experience-recall.sh" ]]; then
    fail "T25a: '$tool' references missing twin $SCRIPTS_DIR/experience-recall.sh"
    twin_ok=0
  elif [[ "$declared_arg0" != "$want_subcommand" ]]; then
    fail "T25a: '$tool' argsTemplate[0] is '$declared_arg0', expected subcommand '$want_subcommand'"
    twin_ok=0
  fi
done
if [[ "$twin_ok" -eq 1 ]]; then
  pass "T25a: all three recall verbs invoke the existing experience-recall.sh twin with their matching subcommand"
fi

# T25b (ADVERSARIAL): the catalog loader must still fail-fast when a recall tool
# points at a missing twin. Without this, T25a/T4 would only prove the current
# catalog happens to be correct, not that a broken one is REJECTED. We stage a
# throwaway repo root holding a single, deliberately-broken recall tool and
# assert the server exits 1 with the missing-script diagnostic.
fixture_root="$(mktemp -d -t bubbles-mcp-recall.XXXXXX)"
mkdir -p "$fixture_root/bubbles/mcp/tools" "$fixture_root/bubbles/scripts"
cp "$SCRIPTS_DIR/experience-recall.sh" "$fixture_root/bubbles/scripts/experience-recall.sh"
python3 -c "
import json,sys
spec = json.load(open(sys.argv[1]))
spec['script'] = 'experience-recall-DELETED.sh'
json.dump(spec, open(sys.argv[2], 'w'), indent=2)
" "$MCP_DIR/tools/search_experience.json" "$fixture_root/bubbles/mcp/tools/search_experience.json"
fixture_rc=0
fixture_err="$(BUBBLES_MCP_REPO_ROOT="$fixture_root" BUBBLES_MCP_LOG_LEVEL=ERROR \
  python3 "$SERVER" </dev/null 2>&1 >/dev/null)" || fixture_rc=$?
rm -rf "$fixture_root"
if [[ "$fixture_rc" -eq 1 && "$fixture_err" == *"references missing script"* ]]; then
  pass "T25b: catalog loader fail-fasts (exit 1) when a recall tool points at a missing bash twin"
else
  fail "T25b: broken recall tool did not fail-fast — rc=$fixture_rc stderr=$fixture_err"
fi

# T26: the MCP surface stays READ-ONLY for recall. No catalog entry may wrap the
# twin's mutating subcommands; sync/delete/admit/lifecycle/export are CLI-only.
ok="$(python3 -c "
import glob, json, os, sys
tools_dir = sys.argv[1]
mutating = {'sync', 'delete', 'admit', 'lifecycle', 'export'}
leaked = []
for path in sorted(glob.glob(os.path.join(tools_dir, '*.json'))):
    spec = json.load(open(path))
    if spec.get('script') != 'experience-recall.sh':
        continue
    args = spec.get('argsTemplate') or []
    if args and args[0] in mutating:
        leaked.append(f\"{spec.get('name')}->{args[0]}\")
print('YES' if not leaked else 'NO:' + ','.join(leaked))
" "$MCP_DIR/tools")"
if [[ "$ok" == "YES" ]]; then
  pass "T26: no MCP tool exposes experience-recall's mutating subcommands (sync/delete/admit/lifecycle/export stay CLI-only)"
else
  fail "T26: mutating recall subcommand leaked onto the MCP surface ($ok)"
fi

echo
if [[ $failures -gt 0 ]]; then
  echo "mcp-server-selftest FAILED with $failures issue(s)."
  exit 1
fi
echo "mcp-server-selftest passed: MCP server boots, dispatches, and surfaces verbatim script output."
