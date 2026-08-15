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
import errno
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
    # Unbuffered both ways: a child we may have to SIGKILL must not carry its
    # only diagnostic away inside an unflushed stdio buffer.
    env["PYTHONUNBUFFERED"] = "1"
    if token:
        env["BUBBLES_MCP_HTTP_TOKEN"] = token
    if extra_env:
        env.update(extra_env)
    argv = [sys.executable, "-u", SERVER, "--transport", "http", "--host", host]
    if port is not None:
        argv += ["--port", str(port)]
    return subprocess.Popen(
        argv, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env, text=True
    )


class Probe:
    """wait_up outcome. Truthy only when ready; otherwise it records WHY.

    "the child exited" and "the child is alive but never accepted a connection"
    need opposite diagnoses, so a bare bool cannot describe the failure.
    """

    def __init__(self, outcome, host, port):
        self.outcome = outcome  # "ready" | "exited" | "timeout"
        self.host = host
        self.port = port

    def __bool__(self):
        return self.outcome == "ready"


def wait_up(proc, port, deadline_s=30.0, host="127.0.0.1"):
    # Wall-clock bounded: a macOS CI runner needs well over the old ~5s budget.
    end = time.monotonic() + deadline_s
    while time.monotonic() < end:
        if proc.poll() is not None:
            return Probe("exited", host, port)
        c = socket.socket()
        c.settimeout(0.2)
        rc = c.connect_ex((host, port))
        c.close()
        if rc == 0:
            return Probe("ready", host, port)
        time.sleep(0.1)
    return Probe("timeout", host, port)


STREAM_CAP = 2000


def cap(text):
    text = text or ""
    if len(text) <= STREAM_CAP:
        return repr(text)
    return f"{text[:STREAM_CAP]!r} (truncated from {len(text)} chars)"


def drain(proc):
    """Stop a child and return (rc, stdout, stderr) for reporting.

    terminate() before kill() so the child gets a chance to flush. Every step is
    guarded: a diagnostic that raises reports less than no diagnostic at all.
    """
    out = err = ""
    try:
        proc.terminate()
    except Exception:
        pass
    try:
        out, err = proc.communicate(timeout=5)
    except subprocess.TimeoutExpired:
        try:
            proc.kill()
        except Exception:
            pass
        try:
            out, err = proc.communicate(timeout=5)
        except Exception:
            pass
    except Exception:
        pass
    return proc.returncode, out, err


# --- timeout diagnostics ------------------------------------------------------
# "alive but silent and not accepting where we probe" is consistent with BOTH
# "blocked before it ever binds" AND "listening somewhere we never probed", and
# stdout/stderr/rc cannot tell those apart. These ask the OS instead. They run
# ONLY on the timeout path and ONLY before the child is drained, because after
# drain the pid is gone and the question is unanswerable. Every call is bounded
# and guarded: a diagnostic that raises destroys the evidence it exists to
# collect, and one that hangs turns a failing test into a stuck job.
DIAG_CAP = 800
DIAG_CMD_TIMEOUT = 2.0
DIAG_CONNECT_TIMEOUT = 1.0


def diag_cap(text):
    """Flatten to ONE line (annotation detail is line-based) and cap it."""
    flat = " | ".join(s.strip() for s in (text or "").splitlines() if s.strip())
    if not flat:
        return "<empty>"
    if len(flat) <= DIAG_CAP:
        return flat
    return f"{flat[:DIAG_CAP]} <TRUNCATED from {len(flat)} chars>"


def diag_run(argv):
    """Bounded best-effort exec. Returns (tool_present, stdout, stderr)."""
    try:
        r = subprocess.run(
            argv, capture_output=True, text=True, timeout=DIAG_CMD_TIMEOUT
        )
    except FileNotFoundError:
        return False, "", ""
    except Exception as exc:
        return True, "", f"<{type(exc).__name__}>"
    return True, r.stdout or "", r.stderr or ""


def diag_listen(pid, port):
    """What does the OS say this pid is listening on? Names the tool that answered.

    stdout carries the answer and stderr only carries noise (lsof warns loudly
    about unstattable mounts), so they are kept apart -- a chatty warning must
    not push the one line we came for past the cap.
    """
    for name, argv, keep in (
        ("lsof", ["lsof", "-w", "-nP", "-p", str(pid), "-a", "-iTCP", "-sTCP:LISTEN"], None),
        ("ss", ["ss", "-ltnp"], f"pid={pid}"),
        ("netstat-ltnp", ["netstat", "-ltnp"], f"{pid}/"),
        ("netstat-anv:port-filtered", ["netstat", "-anv"], f".{port}"),
    ):
        present, out, err = diag_run(argv)
        if not present:
            continue
        if keep is not None:
            out = "\n".join(ln for ln in out.splitlines() if keep in ln)
        if out.strip():
            return f"listen({name})={diag_cap(out)}"
        return f"listen({name})=<NO LISTENING SOCKET> stderr={diag_cap(err)}"
    return "listen(<no tool available: lsof, ss and netstat all absent>)=<unknown>"


def diag_connect(host, port):
    """Connect probe that names the resolved address and classifies the refusal."""
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
        s.settimeout(DIAG_CONNECT_TIMEOUT)
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


def diag_ps(pid):
    """Cheap liveness/state hint. wchan is unavailable on some platforms."""
    for argv in (
        ["ps", "-o", "pid,stat,wchan,command", "-p", str(pid)],
        ["ps", "-o", "pid,stat,command", "-p", str(pid)],
        ["ps", "-p", str(pid)],
    ):
        present, out, _err = diag_run(argv)
        if not present:
            return "ps=<ps not found>"
        rows = [ln for ln in out.splitlines() if ln.strip()][1:]
        if rows:
            return f"ps={diag_cap(chr(10).join(rows))}"
    return "ps=<no output>"


def timeout_diagnostics(proc, port):
    lines = []
    try:
        pid = proc.pid
        alive = "yes" if proc.poll() is None else "no"
        lines.append(f"  FAIL-DIAG: pid={pid} port={port} alive={alive}")
        lines.append(f"  FAIL-DIAG: {diag_listen(pid, port)}")
        fams = " ".join(
            f"{h}={diag_connect(h, port)}" for h in ("127.0.0.1", "::1", "localhost")
        )
        lines.append(f"  FAIL-DIAG: connect {fams}")
        lines.append(f"  FAIL-DIAG: {diag_ps(pid)}")
    except Exception as exc:
        lines.append(f"  FAIL-DIAG: <diagnostic failed: {type(exc).__name__}: {exc}>")
    return "\n".join(lines)


def launch_failure(label, proc, probe):
    diag = ""
    try:
        if probe.outcome == "timeout" and proc.poll() is None:
            diag = timeout_diagnostics(proc, probe.port)
    except Exception:
        pass
    rc, out, err = drain(proc)
    msg = (
        f"{label} did not come up: outcome={probe.outcome} "
        f"probe={probe.host}:{probe.port} rc={rc} "
        f"stdout={cap(out)} stderr={cap(err)}"
    )
    return f"{msg}\n{diag}" if diag else msg


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
    probe = wait_up(proc, schema_port)
    if not probe:
        bad(launch_failure("live 127.0.0.1 tokenless", proc, probe))
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
    probe = wait_up(proc, body_port)
    if not probe:
        bad(launch_failure("body-cap server", proc, probe))
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
