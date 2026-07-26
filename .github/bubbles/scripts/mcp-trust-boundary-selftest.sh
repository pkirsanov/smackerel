#!/usr/bin/env bash
#
# mcp-trust-boundary-selftest.sh — hermetic selftest for the MCP server trust
# boundary (IMP-102 / SCOPE-7). Closes the four defects in bubbles/mcp/server.py
# + bubbles/mcp/tools/record_evidence.json:
#
#   defect #1  Tokenless non-loopback HTTP bind is REFUSED (an unauthenticated
#              tools/call surface executes bash scripts; binding it to a
#              reachable interface is remote code execution). Loopback
#              (127.0.0.1 / ::1 / localhost) stays tokenless for local dev; a
#              BUBBLES_MCP_HTTP_TOKEN lifts the restriction.
#   defect #2  Every tools/call is validated against its inputSchema
#              (additionalProperties:false + required + type). Unknown keys are
#              rejected with a JSON-RPC invalid-params error, NOT executed.
#   defect #3  record_evidence renders ${args*} into argv AND routes
#              spec/scope/tags into BUBBLES_SPEC / BUBBLES_SCOPE /
#              BUBBLES_TOOL_LOG_TAGS via envTemplate on the wrapped run.
#   defect #4  An oversized Content-Length is rejected (413) WITHOUT reading
#              the body (memory-exhaustion DoS guard).
#
# Non-tautology: the bind-refusal and record_evidence-args cases are compared
# against the pre-fix blob (git show 1269bf0:...) — graceful SKIP downstream
# where that SHA is unreachable — and the record_evidence expansion is proven
# to DROP the arg under the old ["--","${command}"] template shape.
#
# Assertions run as a single hermetic Python block (stdlib only). Graceful
# SKIP when python3 is absent; jsonschema-specific asserts SKIP when jsonschema
# is unimportable. Prints "N passed, M failed"; exit nonzero on any failure.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
SERVER="$REPO_ROOT/bubbles/mcp/server.py"

if ! command -v python3 >/dev/null 2>&1; then
  echo "mcp-trust-boundary-selftest: SKIP (python3 not installed)"
  exit 0
fi
if [[ ! -f "$SERVER" ]]; then
  echo "mcp-trust-boundary-selftest: SKIP (server.py not found)"
  exit 0
fi

SELFTEST_REPO_ROOT="$REPO_ROOT" SELFTEST_OLD_SHA="1269bf0" python3 - <<'PY'
import importlib.util
import json
import logging
import os
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

REPO_ROOT = os.environ["SELFTEST_REPO_ROOT"]
OLD_SHA = os.environ.get("SELFTEST_OLD_SHA", "").strip()
SERVER = os.path.join(REPO_ROOT, "bubbles", "mcp", "server.py")
TOOLS = os.path.join(REPO_ROOT, "bubbles", "mcp", "tools")
RECORD_EVIDENCE = os.path.join(TOOLS, "record_evidence.json")
CHECK_GATE = os.path.join(TOOLS, "check_gate.json")

_passed = 0
_failed = 0


def ok(msg):
    global _passed
    _passed += 1
    print(f"  PASS: {msg}")


def bad(msg):
    global _failed
    _failed += 1
    print(f"  FAIL: {msg}")


def skip(msg):
    print(f"  SKIP: {msg}")


# --- import the server module under test (no side effects; main() is guarded) -
spec = importlib.util.spec_from_file_location("bubbles_mcp_server_under_test", SERVER)
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)

_null_logger = logging.getLogger("mcp-trust-boundary-selftest")
_null_logger.addHandler(logging.NullHandler())


def free_port():
    s = socket.socket()
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
    s.close()
    return port


def launch(host, token=None, extra_env=None, port=None):
    env = dict(os.environ)
    env.pop("BUBBLES_MCP_HTTP_TOKEN", None)
    if token:
        env["BUBBLES_MCP_HTTP_TOKEN"] = token
    if extra_env:
        env.update(extra_env)
    argv = [sys.executable, SERVER, "--transport", "http", "--host", host]
    if port is not None:
        argv += ["--port", str(port)]
    return subprocess.Popen(
        argv, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env, text=True
    )


def wait_up(proc, port, tries=50):
    for _ in range(tries):
        if proc.poll() is not None:
            return False
        c = socket.socket()
        c.settimeout(0.2)
        rc = c.connect_ex(("127.0.0.1", port))
        c.close()
        if rc == 0:
            return True
        time.sleep(0.1)
    return False


def stop(proc):
    proc.terminate()
    try:
        proc.communicate(timeout=5)
    except subprocess.TimeoutExpired:
        proc.kill()
        proc.communicate()


def git_show(sha, path):
    if not sha:
        return None
    try:
        r = subprocess.run(
            ["git", "-C", REPO_ROOT, "show", f"{sha}:{path}"],
            capture_output=True, text=True, timeout=15,
        )
    except Exception:
        return None
    return r.stdout if r.returncode == 0 else None


# ===========================================================================
# defect #1 — tokenless non-loopback bind refusal (direct guard functions)
# ===========================================================================
try:
    assert mod._is_loopback_host("127.0.0.1") is True
    assert mod._is_loopback_host("::1") is True
    assert mod._is_loopback_host("localhost") is True
    assert mod._is_loopback_host("0.0.0.0") is False
    assert mod._is_loopback_host("192.168.1.5") is False
    ok("_is_loopback_host classifies loopback vs non-loopback")
except Exception as exc:
    bad(f"_is_loopback_host: {exc}")

try:
    r_refuse = mod._http_bind_refusal_reason("0.0.0.0", "")
    r_loop = mod._http_bind_refusal_reason("127.0.0.1", "")
    r_token = mod._http_bind_refusal_reason("0.0.0.0", "tok")
    assert isinstance(r_refuse, str) and "refusing to bind" in r_refuse
    assert r_loop is None
    assert r_token is None
    # Non-tautology: the three inputs must produce DIFFERENT outcomes, so a
    # no-op guard (always None) would fail the first assertion.
    ok("_http_bind_refusal_reason: refuse(0.0.0.0,no-token) / allow(127.0.0.1) / allow(0.0.0.0,token)")
except Exception as exc:
    bad(f"_http_bind_refusal_reason: {exc}")

# Live: tokenless 0.0.0.0 must REFUSE to start (nonzero exit; does not bind).
proc = launch("0.0.0.0")
try:
    out, err = proc.communicate(timeout=15)
    rc = proc.returncode
    if rc != 0 and "refusing to bind" in (err or ""):
        ok(f"live: --host 0.0.0.0 tokenless REFUSES to start (rc={rc})")
    else:
        bad(f"live 0.0.0.0 tokenless: rc={rc} stderr={err!r}")
except subprocess.TimeoutExpired:
    stop(proc)
    bad("live 0.0.0.0 tokenless did not exit (should refuse immediately)")

# Live: tokenless 127.0.0.1 must still come up, and serve schema-validated calls.
schema_port = free_port()
proc = launch("127.0.0.1", port=schema_port)
try:
    if not wait_up(proc, schema_port):
        _, err = proc.communicate(timeout=5)
        bad(f"live 127.0.0.1 tokenless did not come up; stderr={err!r}")
    else:
        ok("live: --host 127.0.0.1 tokenless starts (loopback stays tokenless)")

        # =============================================================
        # defect #2 — inputSchema validation over a real tools/call
        # =============================================================
        def rpc(payload):
            data = json.dumps(payload).encode()
            req = urllib.request.Request(
                f"http://127.0.0.1:{schema_port}/", data=data, method="POST",
                headers={"Content-Type": "application/json"},
            )
            with urllib.request.urlopen(req, timeout=30) as resp:
                return json.loads(resp.read().decode())

        rej = rpc({"jsonrpc": "2.0", "id": 1, "method": "tools/call",
                   "params": {"name": "check_gate",
                              "arguments": {"action": "list", "bogusKey": 1}}})
        if rej.get("error", {}).get("code") == mod.ERR_INVALID_PARAMS:
            ok("live: tools/call with unknown arg key REJECTED (invalid params), not executed")
        else:
            bad(f"live schema reject: expected ERR_INVALID_PARAMS, got {rej}")

        good = rpc({"jsonrpc": "2.0", "id": 2, "method": "tools/call",
                    "params": {"name": "check_gate", "arguments": {"action": "list"}}})
        if "result" in good and "error" not in good:
            ok("live: valid tools/call passes validation and executes")
        else:
            bad(f"live schema valid: expected a result, got {good}")
except Exception as exc:
    bad(f"live 127.0.0.1 schema block: {exc}")
finally:
    stop(proc)

# Direct schema validation (robust even if the live path is unavailable).
try:
    check_gate = json.load(open(CHECK_GATE))
except Exception as exc:  # pragma: no cover - catalog must exist
    check_gate = None
    bad(f"could not load check_gate.json: {exc}")

if check_gate is not None:
    try:
        mod._validate_arguments(check_gate, {"action": "list", "bogusKey": 1})
        bad("direct _validate_arguments did NOT reject an unknown key")
    except mod._JsonRpcError as exc:
        if exc.code == mod.ERR_INVALID_PARAMS:
            ok("direct: _validate_arguments rejects unknown key (invalid params)")
        else:
            bad(f"direct validate: wrong error code {exc.code}")
    except Exception as exc:
        bad(f"direct validate raised unexpected {type(exc).__name__}: {exc}")

    try:
        mod._validate_arguments(check_gate, {"action": "list"})
        ok("direct: _validate_arguments accepts a valid call")
    except Exception as exc:
        bad(f"direct validate rejected a valid call: {exc}")

    try:
        import jsonschema  # noqa: F401
        have_jsonschema = True
    except ImportError:
        have_jsonschema = False
    if have_jsonschema and check_gate is not None:
        try:
            mod._validate_arguments(check_gate, {"gate_id": "not-a-gate"})
            bad("jsonschema: invalid gate_id pattern not rejected")
        except mod._JsonRpcError:
            ok("jsonschema: invalid gate_id pattern rejected")
        except Exception as exc:
            bad(f"jsonschema pattern check raised {type(exc).__name__}: {exc}")
    else:
        skip("jsonschema-specific pattern assert (jsonschema not importable)")

# ===========================================================================
# defect #3 — record_evidence renders args + env
# ===========================================================================
try:
    rec = json.load(open(RECORD_EVIDENCE))
except Exception as exc:  # pragma: no cover - catalog must exist
    rec = None
    bad(f"could not load record_evidence.json: {exc}")

if rec is not None:
    rendered = mod._render_args(rec.get("argsTemplate", []),
                                {"command": "echo", "args": ["hi"]})
    if "echo" in rendered and "hi" in rendered:
        ok(f"record_evidence argsTemplate expands ${{args*}} -> includes 'hi' ({rendered})")
    else:
        bad(f"record_evidence args expansion missing 'hi': {rendered}")

    # Non-tautology: the OLD template shape drops the wrapped args entirely.
    old_rendered = mod._render_args(["--", "${command}"],
                                    {"command": "echo", "args": ["hi"]})
    if "hi" not in old_rendered:
        ok(f"non-tautology: old-shape argsTemplate ['--','${{command}}'] DROPS 'hi' ({old_rendered})")
    else:
        bad(f"non-tautology broken: old template unexpectedly kept 'hi' ({old_rendered})")

    env_tmpl = rec.get("envTemplate", {})
    spec_val = mod._render_env_value(env_tmpl.get("BUBBLES_SPEC", ""),
                                     {"spec": "demo-spec"})
    if spec_val == "demo-spec":
        ok("record_evidence envTemplate renders BUBBLES_SPEC=demo-spec")
    else:
        bad(f"envTemplate BUBBLES_SPEC render wrong: {spec_val!r}")

    # End-to-end through _execute_tool with a fake tool-log that echoes its
    # argv + BUBBLES_SPEC. Proves BOTH the arg expansion AND the env passing
    # reach the wrapped subprocess (the real record_evidence argsTemplate +
    # envTemplate, only _scriptAbsPath swapped for the fake echo).
    with tempfile.TemporaryDirectory() as td:
        echo = os.path.join(td, "fake-tool-log.sh")
        with open(echo, "w") as fh:
            fh.write(
                "#!/usr/bin/env bash\n"
                'echo "ARGV: $*"\n'
                'echo "BUBBLES_SPEC=${BUBBLES_SPEC-}"\n'
            )
        fake_spec = dict(rec)
        fake_spec["_scriptAbsPath"] = echo
        result = mod._execute_tool(
            fake_spec,
            {"command": "echo", "args": ["hi"], "spec": "demo-spec"},
            REPO_ROOT, _null_logger,
        )
        text = result.get("content", [{}])[0].get("text", "")
        if (not result.get("isError")) and ("hi" in text) and ("BUBBLES_SPEC=demo-spec" in text):
            ok("end-to-end: _execute_tool passes 'hi' arg AND BUBBLES_SPEC=demo-spec env to the wrapped run")
        else:
            bad(f"end-to-end _execute_tool: isError={result.get('isError')} text={text!r}")

# ===========================================================================
# defect #4 — oversized Content-Length rejected without reading the body
# ===========================================================================
body_port = free_port()
proc = launch("127.0.0.1", extra_env={"BUBBLES_MCP_MAX_BODY_BYTES": "64"}, port=body_port)
try:
    if not wait_up(proc, body_port):
        _, err = proc.communicate(timeout=5)
        bad(f"body-cap server did not come up; stderr={err!r}")
    else:
        s = socket.socket()
        s.settimeout(5)
        s.connect(("127.0.0.1", body_port))
        # Declare a huge body but send NONE: if the server read Content-Length
        # bytes it would block forever; the fix returns 413 before reading.
        req = (
            "POST / HTTP/1.0\r\n"
            "Host: 127.0.0.1\r\n"
            "Content-Type: application/json\r\n"
            "Content-Length: 100000000\r\n"
            "Connection: close\r\n\r\n"
        ).encode()
        s.sendall(req)
        resp = b""
        try:
            while True:
                chunk = s.recv(4096)
                if not chunk:
                    break
                resp += chunk
        except socket.timeout:
            pass
        s.close()
        head = resp.decode("latin-1", "replace").split("\r\n", 1)[0]
        if "413" in head:
            ok(f"live: oversized Content-Length rejected with 413 WITHOUT reading body ({head})")
        else:
            bad(f"body-cap: expected 413, got {head!r} (raw={resp[:160]!r})")
except Exception as exc:
    bad(f"body-cap block: {exc}")
finally:
    stop(proc)

# ===========================================================================
# git-differential non-tautology (graceful skip downstream / shallow clones)
# ===========================================================================
old_server = git_show(OLD_SHA, "bubbles/mcp/server.py")
old_rec = git_show(OLD_SHA, "bubbles/mcp/tools/record_evidence.json")
if old_server is None or old_rec is None:
    skip(f"git-differential non-tautology (blob {OLD_SHA or '<unset>'} unreachable)")
else:
    if "_http_bind_refusal_reason" not in old_server:
        ok(f"non-tautology(git): old server.py @ {OLD_SHA} has NO bind guard (fix is genuinely new)")
    else:
        bad(f"non-tautology(git): old server.py @ {OLD_SHA} already had _http_bind_refusal_reason")
    try:
        old_rec_json = json.loads(old_rec)
    except Exception as exc:
        old_rec_json = {}
        bad(f"could not parse old record_evidence.json: {exc}")
    old_tmpl = old_rec_json.get("argsTemplate", [])
    if "${args*}" not in old_tmpl and "envTemplate" not in old_rec_json:
        ok(f"non-tautology(git): old record_evidence.json @ {OLD_SHA} drops args (no ${{args*}}, no envTemplate)")
    else:
        bad(f"non-tautology(git): old record_evidence.json @ {OLD_SHA} unexpectedly had args/env expansion")

print("")
print(f"[mcp-trust-boundary-selftest] {_passed} passed, {_failed} failed")
sys.exit(1 if _failed else 0)
PY
rc=$?
exit "$rc"
