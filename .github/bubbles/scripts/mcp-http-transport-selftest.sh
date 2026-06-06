#!/usr/bin/env bash
#
# mcp-http-transport-selftest.sh — hermetic selftest for the v6.1 (R9) MCP
# HTTP transport. Boots the server with --transport http on an ephemeral port
# and verifies JSON-RPC over HTTP POST, the health probe, and bearer auth.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
SERVER="$REPO_ROOT/bubbles/mcp/server.py"

if ! command -v python3 >/dev/null 2>&1; then
  echo "mcp-http-transport-selftest: SKIP (python3 not installed)"
  exit 0
fi
if [[ ! -f "$SERVER" ]]; then
  echo "mcp-http-transport-selftest: SKIP (server.py not found)"
  exit 0
fi

pass_count=0
fail_count=0
pass() { echo "  PASS: $1"; pass_count=$((pass_count + 1)); }
fail() { echo "  FAIL: $1"; fail_count=$((fail_count + 1)); }

# Pick a free ephemeral port via python.
PORT="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')"
TOKEN="selftest-token-123"
LOG="$(mktemp)"

# Start the HTTP transport with auth enabled.
BUBBLES_MCP_HTTP_TOKEN="$TOKEN" BUBBLES_MCP_LOG_FILE="$LOG" \
  python3 "$SERVER" --transport http --host 127.0.0.1 --port "$PORT" &
SERVER_PID=$!
cleanup() { kill "$SERVER_PID" 2>/dev/null || true; wait "$SERVER_PID" 2>/dev/null || true; rm -f "$LOG"; }
trap cleanup EXIT INT TERM

# Wait for the port to accept connections (up to ~5s).
ready=0
for _ in $(seq 1 50); do
  if python3 -c "import socket,sys; s=socket.socket(); s.settimeout(0.2)
sys.exit(0 if s.connect_ex(('127.0.0.1',$PORT))==0 else 1)" 2>/dev/null; then
    ready=1; break
  fi
  sleep 0.1
done
if [[ "$ready" -eq 1 ]]; then pass "HTTP transport accepts connections on :$PORT"; else fail "server never came up on :$PORT"; cat "$LOG" 2>/dev/null; echo ""; echo "[mcp-http-transport-selftest] $pass_count passed, $((fail_count+1)) failed"; exit 1; fi

# Helper: POST a JSON body, print "HTTP_STATUS\n<body>".
http_post() {
  local body="$1"; shift
  python3 - "$PORT" "$TOKEN" "$body" "$@" <<'PY'
import sys, json, urllib.request
port, token, body = sys.argv[1], sys.argv[2], sys.argv[3]
use_auth = "noauth" not in sys.argv[4:]
req = urllib.request.Request(f"http://127.0.0.1:{port}/", data=body.encode(),
                             method="POST", headers={"Content-Type": "application/json"})
if use_auth:
    req.add_header("Authorization", f"Bearer {token}")
try:
    with urllib.request.urlopen(req, timeout=5) as r:
        print(r.status); print(r.read().decode())
except urllib.error.HTTPError as e:
    print(e.code); print(e.read().decode())
PY
}

# T1: initialize round-trip
init_body='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}'
out="$(http_post "$init_body")"
status="$(printf '%s' "$out" | head -1)"
payload="$(printf '%s' "$out" | tail -n +2)"
if [[ "$status" == "200" ]] && printf '%s' "$payload" | grep -q '"protocolVersion"' && printf '%s' "$payload" | grep -q '"bubbles"'; then
  pass "initialize over HTTP returns protocolVersion + serverInfo"
else
  fail "initialize failed (status=$status): $payload"
fi

# T2: tools/list returns a non-empty catalog
list_body='{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
out="$(http_post "$list_body")"
payload="$(printf '%s' "$out" | tail -n +2)"
tool_count="$(printf '%s' "$payload" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(len(d.get("result",{}).get("tools",[])))' 2>/dev/null || echo 0)"
if [[ "$tool_count" -ge 5 ]]; then
  pass "tools/list over HTTP returns the catalog ($tool_count tools)"
else
  fail "tools/list returned $tool_count tools: $payload"
fi

# T3: health probe
health="$(python3 - "$PORT" <<'PY'
import sys, urllib.request
port = sys.argv[1]
with urllib.request.urlopen(f"http://127.0.0.1:{port}/health", timeout=5) as r:
    print(r.status)
PY
)"
if [[ "$health" == "200" ]]; then pass "GET /health returns 200"; else fail "health probe returned $health"; fi

# T4: missing auth is rejected (401)
out="$(http_post "$init_body" noauth)"
status="$(printf '%s' "$out" | head -1)"
if [[ "$status" == "401" ]]; then pass "request without bearer token is rejected (401)"; else fail "unauthorized request got status $status (expected 401)"; fi

# T5: malformed JSON returns a JSON-RPC parse error, not a crash
out="$(http_post 'not-json{')"
payload="$(printf '%s' "$out" | tail -n +2)"
if printf '%s' "$payload" | grep -q '"error"' && printf '%s' "$payload" | grep -qi 'parse'; then
  pass "malformed body returns a JSON-RPC parse error"
else
  fail "malformed body handling unexpected: $payload"
fi

echo ""
echo "[mcp-http-transport-selftest] $pass_count passed, $fail_count failed"
[[ "$fail_count" -eq 0 ]] || exit 1
echo "[mcp-http-transport-selftest] OK"
exit 0
