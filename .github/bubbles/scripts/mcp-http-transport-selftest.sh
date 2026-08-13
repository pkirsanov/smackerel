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

# Print a captured stream, capped so a chatty server cannot flood the CI
# annotation. python3 is already a hard requirement of this selftest.
dump_capped() {
  python3 - "$1" "$2" <<'PY'
import sys
label, path = sys.argv[1], sys.argv[2]
try:
    with open(path, "r", errors="replace") as fh:
        text = fh.read()
except OSError as exc:
    print(f"    {label}: <unreadable: {exc}>")
    raise SystemExit(0)
cap = 2000
if not text:
    print(f"    {label}: <empty>")
elif len(text) <= cap:
    print(f"    {label}:\n{text}")
else:
    print(f"    {label} (truncated from {len(text)} chars):\n{text[:cap]}")
PY
}

# OS-level view of a child that is ALIVE but never accepted a connection. Runs
# ONLY on that failure path and ONLY before the child is signalled: once it is
# reaped the pid is gone and "blocked before it ever bound" can no longer be
# told apart from "listening somewhere we never probed", which stdout/stderr/rc
# alone cannot distinguish. Strictly best-effort — every call is bounded and
# guarded, and stderr is discarded so a diagnostic traceback can never be
# mistaken for the failure under investigation.
timeout_diagnostics() {
  python3 - "$1" "$2" 2>/dev/null <<'PY' || true
import errno
import socket
import subprocess
import sys

pid = sys.argv[1]
port = int(sys.argv[2])
CAP = 800
CMD_TIMEOUT = 2.0
CONNECT_TIMEOUT = 1.0


def cap1(text):
    """Flatten to ONE line (annotation detail is line-based) and cap it."""
    flat = " | ".join(s.strip() for s in (text or "").splitlines() if s.strip())
    if not flat:
        return "<empty>"
    if len(flat) <= CAP:
        return flat
    return f"{flat[:CAP]} <TRUNCATED from {len(flat)} chars>"


def run(argv):
    try:
        r = subprocess.run(argv, capture_output=True, text=True, timeout=CMD_TIMEOUT)
    except FileNotFoundError:
        return False, "", ""
    except Exception as exc:
        return True, "", f"<{type(exc).__name__}>"
    return True, r.stdout or "", r.stderr or ""


def listen():
    # stdout carries the answer, stderr only noise (lsof warns loudly about
    # unstattable mounts) -- kept apart so a warning cannot push the one line
    # we came for past the cap.
    for name, argv, keep in (
        ("lsof", ["lsof", "-w", "-nP", "-p", pid, "-a", "-iTCP", "-sTCP:LISTEN"], None),
        ("ss", ["ss", "-ltnp"], f"pid={pid}"),
        ("netstat-ltnp", ["netstat", "-ltnp"], f"{pid}/"),
        ("netstat-anv:port-filtered", ["netstat", "-anv"], f".{port}"),
    ):
        present, out, err = run(argv)
        if not present:
            continue
        if keep is not None:
            out = "\n".join(ln for ln in out.splitlines() if keep in ln)
        if out.strip():
            return f"listen({name})={cap1(out)}"
        return f"listen({name})=<NO LISTENING SOCKET> stderr={cap1(err)}"
    return "listen(<no tool available: lsof, ss and netstat all absent>)=<unknown>"


def connect(host):
    try:
        infos = socket.getaddrinfo(host, port, 0, socket.SOCK_STREAM)
    except Exception as exc:
        return f"<resolve-failed:{type(exc).__name__}>"
    if not infos:
        return "<no-address>"
    fam, stype, proto, _canon, sa = infos[0]
    addr = sa[0]
    s = None
    try:
        s = socket.socket(fam, stype, proto)
        s.settimeout(CONNECT_TIMEOUT)
        rc = s.connect_ex(sa)
    except Exception as exc:
        return f"[{addr}]<{type(exc).__name__}>"
    finally:
        if s is not None:
            try:
                s.close()
            except Exception:
                pass
    if rc == 0:
        return f"[{addr}]OPEN"
    return f"[{addr}]{errno.errorcode.get(rc, 'rc')}={rc}"


def psline():
    for argv in (
        ["ps", "-o", "pid,stat,wchan,command", "-p", pid],
        ["ps", "-o", "pid,stat,command", "-p", pid],
        ["ps", "-p", pid],
    ):
        present, out, _err = run(argv)
        if not present:
            return "ps=<ps not found>"
        rows = [ln for ln in out.splitlines() if ln.strip()][1:]
        if rows:
            return f"ps={cap1(chr(10).join(rows))}"
    return "ps=<no output>"


out = []
try:
    out.append(f"  FAIL-DIAG: pid={pid} port={port}")
    out.append(f"  FAIL-DIAG: {listen()}")
    fams = " ".join(f"{h}={connect(h)}" for h in ("127.0.0.1", "::1", "localhost"))
    out.append(f"  FAIL-DIAG: connect {fams}")
    out.append(f"  FAIL-DIAG: {psline()}")
except Exception as exc:
    out.append(f"  FAIL-DIAG: <diagnostic failed: {type(exc).__name__}: {exc}>")
print("\n".join(out))
PY
}

# Pick a free ephemeral port via python.
PORT="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')"
TOKEN="selftest-token-123"
LOG="$(mktemp)"
OUT_LOG="$(mktemp)"
ERR_LOG="$(mktemp)"

# Start the HTTP transport with auth enabled. Unbuffered both ways so a child we
# may have to SIGKILL cannot carry its only diagnostic away in a stdio buffer;
# stdout and stderr are captured so a failure can actually report them.
BUBBLES_MCP_HTTP_TOKEN="$TOKEN" BUBBLES_MCP_LOG_FILE="$LOG" PYTHONUNBUFFERED=1 \
  python3 -u "$SERVER" --transport http --host 127.0.0.1 --port "$PORT" \
  >"$OUT_LOG" 2>"$ERR_LOG" &
SERVER_PID=$!
cleanup() { kill "$SERVER_PID" 2>/dev/null || true; wait "$SERVER_PID" 2>/dev/null || true; rm -f "$LOG" "$OUT_LOG" "$ERR_LOG"; }
trap cleanup EXIT INT TERM

# Wall-clock bounded: a macOS CI runner needs well over the old ~5s budget.
# A dead child breaks out early: "exited" and "alive but never accepted a
# connection" are opposite diagnoses and must not collapse into one outcome.
ready=0
outcome="timeout: child still running, never accepted a connection"
_deadline=$((SECONDS + 30))
while (( SECONDS < _deadline )); do
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    outcome="exited: child died before accepting a connection"
    break
  fi
  if python3 -c "import socket,sys; s=socket.socket(); s.settimeout(0.2)
sys.exit(0 if s.connect_ex(('127.0.0.1',$PORT))==0 else 1)" 2>/dev/null; then
    ready=1; break
  fi
  sleep 0.1
done

if [[ "$ready" -eq 1 ]]; then
  pass "HTTP transport accepts connections on :$PORT"
else
  # Ask the OS about the child BEFORE signalling it; after the reap the pid is
  # gone and the question can no longer be asked.
  _diag=""
  if kill -0 "$SERVER_PID" 2>/dev/null; then
    _diag="$(timeout_diagnostics "$SERVER_PID" "$PORT")"
  fi
  # Graceful stop first so buffered output flushes; SIGKILL only as a fallback.
  if kill -0 "$SERVER_PID" 2>/dev/null; then
    kill -TERM "$SERVER_PID" 2>/dev/null || true
    _stop_deadline=$((SECONDS + 5))
    while (( SECONDS < _stop_deadline )) && kill -0 "$SERVER_PID" 2>/dev/null; do
      sleep 0.1
    done
    kill -KILL "$SERVER_PID" 2>/dev/null || true
  fi
  wait "$SERVER_PID" 2>/dev/null
  server_rc=$?
  fail "server never came up on :$PORT — outcome=$outcome probe=127.0.0.1:$PORT rc=$server_rc"
  if [[ -n "$_diag" ]]; then
    printf '%s\n' "$_diag"
  fi
  dump_capped "server stdout" "$OUT_LOG"
  dump_capped "server stderr" "$ERR_LOG"
  dump_capped "server log file" "$LOG"
  echo ""
  echo "[mcp-http-transport-selftest] $pass_count passed, $fail_count failed"
  exit 1
fi

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
if [[ "$fail_count" -ne 0 ]]; then
  # The server's streams are captured now, so a later-test failure must still
  # surface them rather than losing what used to be inherited output.
  dump_capped "server stdout" "$OUT_LOG"
  dump_capped "server stderr" "$ERR_LOG"
  dump_capped "server log file" "$LOG"
fi
echo "[mcp-http-transport-selftest] $pass_count passed, $fail_count failed"
[[ "$fail_count" -eq 0 ]] || exit 1
echo "[mcp-http-transport-selftest] OK"
exit 0
