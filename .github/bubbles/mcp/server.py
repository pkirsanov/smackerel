#!/usr/bin/env python3
"""
Bubbles MCP server — Model Context Protocol bridge for the Bubbles framework.

Transport: stdio (newline-delimited JSON-RPC 2.0 per MCP spec — one JSON
object per line; a legacy LSP-style `Content-Length:` header form is still
accepted on read for back-compat) OR HTTP (v6.1 / R9 — JSON-RPC over
POST, reachable from CI runners and shared/cloud environments).
Protocol: negotiates MCP 2024-11-05 / 2025-03-26 / 2025-06-18 (echoes the
client's requested version when supported, else returns the latest).
Runtime: Python 3.10+, stdlib only. No pip install. No daemon.

Surface (per docs/v6-mcp-design.md):
  Tools (A2): thin wrappers around bubbles/scripts/*.sh — see
    bubbles/mcp/tools/*.json for the declarative catalog.
  Resources (A3): read-only handles to canonical repo files — see
    bubbles/mcp/resources/*.json.
    Prompts: read-only exposure of the existing VS Code prompt shims under
        prompts/*.prompt.md (or .github/prompts/*.prompt.md downstream).

Design rules (NON-NEGOTIABLE):
  - The server NEVER duplicates business logic. Every tool dispatches to an
    existing bash script under bubbles/scripts/ via subprocess.
  - Resources resolve to file reads against the repo root; no remote fetch.
  - Server is OPTIONAL. Every operator capability that v6 exposes via MCP
    also remains reachable via the underlying bash script (the bash twin).
  - Anti-fabrication: tool output is the script's stdout + stderr + exit
    code, captured verbatim, with the actual command line recorded in the
    response envelope. The server NEVER summarizes, paraphrases, or hides
    script output.

Invocation:
  python3 bubbles/mcp/server.py                       # stdio (default)
  python3 bubbles/mcp/server.py --transport http --host 127.0.0.1 --port 8765
  # OR, for repos that installed Bubbles into .github/:
  python3 .github/bubbles/mcp/server.py

The repo root is auto-detected by walking up from this file until a
sentinel (bubbles/scripts/cli.sh OR .github/bubbles/scripts/cli.sh) is found.

Environment overrides:
  BUBBLES_MCP_REPO_ROOT      absolute path to repo root (skips auto-detect)
  BUBBLES_MCP_TOOL_TIMEOUT   per-tool timeout in seconds (default 300)
  BUBBLES_MCP_LOG_LEVEL      DEBUG|INFO|WARNING|ERROR (default INFO)
  BUBBLES_MCP_LOG_FILE       absolute path for diagnostic log
                             (default: $TMPDIR/bubbles-mcp-server.log)
  BUBBLES_MCP_HTTP_TOKEN     when set, HTTP transport requires
                             `Authorization: Bearer <token>` on POST

Exit codes:
  0   stdin EOF; clean shutdown
  1   fatal startup error (missing tool catalog, repo root not found)
  2   bad CLI argument
"""

import json
import logging
import os
import re
import shutil
import subprocess
import sys
import threading
import time
from pathlib import Path
from typing import Any, Optional

# ---------------------------------------------------------------------------
# Constants

# MCP protocol versions this server can speak, newest first. We negotiate per
# the MCP spec: echo back the client's requested version when we support it,
# otherwise return our latest supported version and let the client decide
# whether to proceed. The wire surface we expose (tools + annotations,
# resources + templates, prompts) is compatible across this range; newer-only
# features are optional capabilities we simply do not advertise.
SUPPORTED_PROTOCOL_VERSIONS = ("2025-06-18", "2025-03-26", "2024-11-05")
PROTOCOL_VERSION = SUPPORTED_PROTOCOL_VERSIONS[0]  # latest MCP version we implement
SERVER_NAME = "bubbles"
# Server version reads from VERSION file when running from source repo;
# falls back to .github/bubbles/.version when running from a downstream
# install. Resolved at startup.
DEFAULT_TOOL_TIMEOUT_SECONDS = 300

# JSON-RPC error codes (per spec):
ERR_PARSE = -32700
ERR_INVALID_REQUEST = -32600
ERR_METHOD_NOT_FOUND = -32601
ERR_INVALID_PARAMS = -32602
ERR_INTERNAL = -32603
# MCP-specific (we use the -32000..-32099 server range)
ERR_TOOL_NOT_FOUND = -32001
ERR_TOOL_FAILED = -32002
ERR_RESOURCE_NOT_FOUND = -32003
ERR_RESOURCE_FAILED = -32004
ERR_PROMPT_NOT_FOUND = -32005


# ---------------------------------------------------------------------------
# Repo-root + version resolution

def _detect_repo_root() -> Path:
    """Walk up from this file until we find a Bubbles repo sentinel.

    Source repo sentinel:    <root>/bubbles/scripts/cli.sh
    Downstream sentinel:     <root>/.github/bubbles/scripts/cli.sh
    """
    override = os.environ.get("BUBBLES_MCP_REPO_ROOT")
    if override:
        p = Path(override).resolve()
        if not p.is_dir():
            sys.stderr.write(f"BUBBLES_MCP_REPO_ROOT does not exist: {p}\n")
            sys.exit(1)
        return p

    here = Path(__file__).resolve()
    for candidate in [here.parent, *here.parents]:
        if (candidate / "bubbles" / "scripts" / "cli.sh").is_file():
            return candidate
        if (candidate / ".github" / "bubbles" / "scripts" / "cli.sh").is_file():
            return candidate
    sys.stderr.write(
        "bubbles-mcp: could not locate repo root from "
        f"{here}; set BUBBLES_MCP_REPO_ROOT.\n"
    )
    sys.exit(1)


def _resolve_scripts_dir(repo_root: Path) -> Path:
    """Return the path to bubbles/scripts within the active install layout."""
    src = repo_root / "bubbles" / "scripts"
    if src.is_dir():
        return src
    downstream = repo_root / ".github" / "bubbles" / "scripts"
    if downstream.is_dir():
        return downstream
    sys.stderr.write(
        f"bubbles-mcp: scripts dir not found at {src} or {downstream}\n"
    )
    sys.exit(1)


def _resolve_mcp_dir(repo_root: Path) -> Path:
    """Return the path to bubbles/mcp within the active install layout."""
    src = repo_root / "bubbles" / "mcp"
    if src.is_dir():
        return src
    downstream = repo_root / ".github" / "bubbles" / "mcp"
    if downstream.is_dir():
        return downstream
    # Allow startup with no catalog — server exposes zero tools/resources
    # but still responds to initialize, ping, and method-not-found queries.
    return src

def _resolve_prompts_dir(repo_root: Path) -> Path:
    """Return the path to Bubbles prompt shims in source/downstream layouts."""
    src = repo_root / "prompts"
    if src.is_dir():
        return src
    downstream = repo_root / ".github" / "prompts"
    if downstream.is_dir():
        return downstream
    return src


def _resolve_server_version(repo_root: Path) -> str:
    """Read VERSION (source repo) or .github/bubbles/.version (downstream)."""
    for candidate in (
        repo_root / "VERSION",
        repo_root / ".github" / "bubbles" / ".version",
        repo_root / "bubbles" / ".version",
    ):
        if candidate.is_file():
            return candidate.read_text(encoding="utf-8").strip()
    return "unknown"


# ---------------------------------------------------------------------------
# JSON-RPC framing (MCP stdio transport)
#
# Per the MCP stdio transport spec, each message is a JSON-RPC 2.0 object
# serialized as newline-delimited JSON: one JSON object per line, terminated
# by a single `\n`. A legacy LSP-style Content-Length header block is still
# accepted on read for back-compat:
#
#   Content-Length: <bytes>\r\n
#   \r\n
#   <payload>
#
# Both stdin and stdout MUST use binary mode so we can count bytes accurately.

class StdioTransport:
    def __init__(self, reader, writer):
        self._reader = reader  # binary
        self._writer = writer  # binary
        self._write_lock = threading.Lock()

    def read_message(self) -> Optional[dict[str, Any]]:
        """Read one JSON-RPC message. Returns None on EOF.

        MCP stdio transport is newline-delimited JSON (one JSON object per
        line). A legacy LSP-style Content-Length header block is still
        accepted on read for back-compat.
        """
        line = self._reader.readline()
        if not line:
            return None  # EOF
        stripped = line.strip()
        while not stripped:  # skip blank lines between messages
            line = self._reader.readline()
            if not line:
                return None  # EOF
            stripped = line.strip()
        # Newline-delimited JSON (MCP stdio spec): a JSON object, not a header.
        if not stripped.lower().startswith(b"content-length:"):
            return json.loads(stripped.decode("utf-8"))
        # Legacy Content-Length framing: parse remaining header block + body.
        content_length = int(stripped.partition(b":")[2].strip())
        while True:
            header_line = self._reader.readline()
            if not header_line or not header_line.strip():
                break  # blank line ends the header block (or EOF)
        body = self._reader.read(content_length)
        if len(body) != content_length:
            raise ValueError(
                f"short read: expected {content_length} bytes, got {len(body)}"
            )
        return json.loads(body.decode("utf-8"))

    def write_message(self, msg: dict[str, Any]) -> None:
        payload = json.dumps(msg, separators=(",", ":")).encode("utf-8")
        with self._write_lock:
            self._writer.write(payload + b"\n")
            self._writer.flush()


# ---------------------------------------------------------------------------
# Tool + resource catalog loading

class ToolCatalog:
    """Loads tool definitions from bubbles/mcp/tools/*.json.

    Each tool JSON has the shape:
      {
        "name": "validate_dod",
        "description": "...",
        "inputSchema": { ... JSON Schema ... },
        "script": "state-transition-guard.sh",     # relative to scripts dir
        "argsTemplate": ["--specDir", "${spec_dir}"],
        "successExitCodes": [0],
        "timeoutSeconds": 300                       # optional override
      }

    Bash twin guarantee (v6 design rule): every tool MUST reference an
    existing script under scripts_dir. Loader REJECTS catalog entries that
    point to a missing script (fail fast at startup).
    """

    def __init__(self, mcp_dir: Path, scripts_dir: Path, logger: logging.Logger):
        self._mcp_dir = mcp_dir
        self._scripts_dir = scripts_dir
        self._tools: dict[str, dict[str, Any]] = {}
        self._logger = logger
        self._reload()

    def _reload(self) -> None:
        tools_dir = self._mcp_dir / "tools"
        if not tools_dir.is_dir():
            self._logger.info("tool catalog empty: %s not found", tools_dir)
            return
        for path in sorted(tools_dir.glob("*.json")):
            try:
                spec = json.loads(path.read_text(encoding="utf-8"))
            except json.JSONDecodeError as exc:
                sys.stderr.write(
                    f"bubbles-mcp: catalog file {path} is not valid JSON: {exc}\n"
                )
                sys.exit(1)
            name = spec.get("name")
            script = spec.get("script")
            if not name or not script:
                sys.stderr.write(
                    f"bubbles-mcp: catalog file {path} missing name/script\n"
                )
                sys.exit(1)
            script_path = self._scripts_dir / script
            if not script_path.is_file():
                sys.stderr.write(
                    f"bubbles-mcp: tool '{name}' references missing script "
                    f"{script_path}\n"
                )
                sys.exit(1)
            spec.setdefault("description", "")
            spec.setdefault("inputSchema", {"type": "object"})
            spec.setdefault("argsTemplate", [])
            spec.setdefault("successExitCodes", [0])
            spec.setdefault("timeoutSeconds", DEFAULT_TOOL_TIMEOUT_SECONDS)
            spec["_scriptAbsPath"] = str(script_path.resolve())
            self._tools[name] = spec
            self._logger.debug("loaded tool: %s -> %s", name, script_path)

    def list_tools(self) -> list[dict[str, Any]]:
        out = []
        for name, spec in self._tools.items():
            entry = {
                "name": name,
                "description": spec.get("description", ""),
                "inputSchema": spec.get("inputSchema", {"type": "object"}),
            }
            if "annotations" in spec:
                entry["annotations"] = spec["annotations"]
            out.append(entry)
        return out

    def get(self, name: str) -> Optional[dict[str, Any]]:
        return self._tools.get(name)


class ResourceCatalog:
    """Loads resource definitions from bubbles/mcp/resources/*.json.

    Two kinds of resource are supported:

    1. STATIC — a fixed `uri` mapped to a repo-relative `path`:
         {
           "uri": "bubbles://workflows.yaml",
           "name": "...", "description": "...",
           "mimeType": "application/yaml",
           "path": "bubbles/workflows.yaml"
         }

    2. TEMPLATED — a `uriTemplate` (RFC 6570 level-1 `{var}` expansion) that
       expands to a FAMILY of resources. A templated resource resolves its
       content one of two ways:

       a. `pathTemplate` — a repo-relative path/glob with `{var}` placeholders.
            { "uriTemplate": "bubbles://spec/{nnn}/state.json",
              "pathTemplate": "specs/{nnn}*/state.json", ... }

       b. `commandTemplate` — a bash twin under scripts_dir, run with
          `{var}`-substituted args; stdout becomes the resource body. This keeps
          the server a THIN wrapper — it never duplicates the bash twin's logic.
            { "uriTemplate": "bubbles://gates/{id}",
              "commandTemplate": {"script": "gate-meta.sh", "args": ["json", "{id}"]}, ... }

    Static URIs match first; if none match, the URI is tested against each
    template's `uriTemplate`. Extracted variables may not contain `..` and
    (being single URI path segments) never contain `/`.
    """

    def __init__(
        self,
        mcp_dir: Path,
        repo_root: Path,
        scripts_dir: Path,
        logger: logging.Logger,
    ):
        self._mcp_dir = mcp_dir
        self._repo_root = repo_root
        self._scripts_dir = scripts_dir
        self._logger = logger
        self._resources: dict[str, dict[str, Any]] = {}
        self._templates: list[dict[str, Any]] = []
        self._reload()

    @staticmethod
    def _compile_uri_template(uri_template: str) -> "re.Pattern[str]":
        # RFC 6570 level-1 simple expansion: {var} -> exactly one path segment.
        parts: list[str] = []
        idx = 0
        while idx < len(uri_template):
            start = uri_template.find("{", idx)
            if start == -1:
                parts.append(re.escape(uri_template[idx:]))
                break
            parts.append(re.escape(uri_template[idx:start]))
            end = uri_template.find("}", start + 1)
            if end == -1:
                parts.append(re.escape(uri_template[start:]))
                break
            var = uri_template[start + 1 : end]
            parts.append(f"(?P<{var}>[^/]+)")
            idx = end + 1
        return re.compile("^" + "".join(parts) + "$")

    def _reload(self) -> None:
        res_dir = self._mcp_dir / "resources"
        if not res_dir.is_dir():
            self._logger.info("resource catalog empty: %s not found", res_dir)
            return
        for path in sorted(res_dir.glob("*.json")):
            try:
                spec = json.loads(path.read_text(encoding="utf-8"))
            except json.JSONDecodeError as exc:
                sys.stderr.write(
                    f"bubbles-mcp: resource file {path} is not valid JSON: {exc}\n"
                )
                sys.exit(1)
            uri = spec.get("uri")
            uri_template = spec.get("uriTemplate")
            if not uri and not uri_template:
                sys.stderr.write(
                    f"bubbles-mcp: resource file {path} missing uri/uriTemplate\n"
                )
                sys.exit(1)
            spec.setdefault("description", "")
            spec.setdefault("mimeType", "text/plain")
            if uri_template:
                spec.setdefault("name", uri_template)
                if "pathTemplate" not in spec and "commandTemplate" not in spec:
                    sys.stderr.write(
                        f"bubbles-mcp: templated resource {uri_template} needs "
                        "pathTemplate or commandTemplate\n"
                    )
                    sys.exit(1)
                spec["_uriRegex"] = self._compile_uri_template(uri_template)
                self._templates.append(spec)
                self._logger.debug("loaded resource template: %s", uri_template)
            else:
                spec.setdefault("name", uri)
                self._resources[uri] = spec
                self._logger.debug("loaded resource: %s", uri)

    def list_resources(self) -> list[dict[str, Any]]:
        out = []
        for uri, spec in self._resources.items():
            entry = {
                "uri": uri,
                "name": spec.get("name", uri),
                "description": spec.get("description", ""),
                "mimeType": spec.get("mimeType", "text/plain"),
            }
            out.append(entry)
        return out

    def list_resource_templates(self) -> list[dict[str, Any]]:
        out = []
        for spec in self._templates:
            out.append(
                {
                    "uriTemplate": spec["uriTemplate"],
                    "name": spec.get("name", spec["uriTemplate"]),
                    "description": spec.get("description", ""),
                    "mimeType": spec.get("mimeType", "text/plain"),
                }
            )
        return out

    def read(self, uri: str) -> Optional[dict[str, Any]]:
        spec = self._resources.get(uri)
        if spec is not None:
            return self._read_static(uri, spec)
        for tspec in self._templates:
            match = tspec["_uriRegex"].match(uri)
            if not match:
                continue
            variables = match.groupdict()
            for var, val in variables.items():
                if ".." in val:
                    return {"error": f"resource {uri} variable '{var}' contains '..'"}
            if "commandTemplate" in tspec:
                return self._read_command(uri, tspec, variables)
            return self._read_path_template(uri, tspec, variables)
        return None

    def _read_static(self, uri: str, spec: dict[str, Any]) -> dict[str, Any]:
        rel = spec.get("path")
        if not rel:
            return {"error": f"resource {uri} has no path"}
        return self._read_file(uri, rel, spec.get("mimeType", "text/plain"))

    def _read_path_template(
        self, uri: str, spec: dict[str, Any], variables: dict[str, str]
    ) -> dict[str, Any]:
        pattern = spec["pathTemplate"]
        for var, val in variables.items():
            pattern = pattern.replace("{" + var + "}", val)
        matches = [p for p in sorted(self._repo_root.glob(pattern)) if p.is_file()]
        if not matches:
            return {"error": f"resource {uri} matched no file (pattern: {pattern})"}
        if len(matches) > 1:
            rels = ", ".join(str(p.relative_to(self._repo_root)) for p in matches)
            return {"error": f"resource {uri} is ambiguous (matched: {rels})"}
        rel = str(matches[0].relative_to(self._repo_root))
        return self._read_file(uri, rel, spec.get("mimeType", "text/plain"))

    def _read_file(self, uri: str, rel: str, mime: str) -> dict[str, Any]:
        abs_path = (self._repo_root / rel).resolve()
        # Containment check: don't allow `..` traversal out of repo_root.
        try:
            abs_path.relative_to(self._repo_root.resolve())
        except ValueError:
            return {"error": f"resource {uri} path escapes repo_root"}
        if not abs_path.is_file():
            return {"error": f"resource {uri} file not found: {abs_path}"}
        try:
            text = abs_path.read_text(encoding="utf-8")
        except UnicodeDecodeError:
            return {"error": f"resource {uri} is not UTF-8 text"}
        return {"text": text, "mimeType": mime}

    def _read_command(
        self, uri: str, spec: dict[str, Any], variables: dict[str, str]
    ) -> dict[str, Any]:
        cmd_spec = spec["commandTemplate"]
        script = cmd_spec.get("script")
        if not script:
            return {"error": f"resource {uri} commandTemplate missing script"}
        script_path = (self._scripts_dir / script).resolve()
        # Containment: the script must live under scripts_dir.
        try:
            script_path.relative_to(self._scripts_dir.resolve())
        except ValueError:
            return {"error": f"resource {uri} script escapes scripts_dir"}
        if not script_path.is_file():
            return {"error": f"resource {uri} script not found: {script}"}
        args: list[str] = []
        for tok in cmd_spec.get("args", []):
            rendered = tok
            for var, val in variables.items():
                rendered = rendered.replace("{" + var + "}", val)
            args.append(rendered)
        cmd = ["bash", str(script_path), *args]
        timeout = int(
            os.environ.get("BUBBLES_MCP_TOOL_TIMEOUT", DEFAULT_TOOL_TIMEOUT_SECONDS)
        )
        try:
            completed = subprocess.run(
                cmd,
                cwd=self._repo_root,
                capture_output=True,
                text=True,
                timeout=timeout,
                check=False,
            )
        except subprocess.TimeoutExpired:
            return {"error": f"resource {uri} command timed out after {timeout}s"}
        if completed.returncode != 0:
            return {
                "error": (
                    f"resource {uri} command exit={completed.returncode}: "
                    f"{completed.stderr.strip()}"
                )
            }
        return {
            "text": completed.stdout,
            "mimeType": spec.get("mimeType", "text/plain"),
        }


class PromptCatalog:
    """Loads Bubbles prompt shims from prompts/*.prompt.md.

    Prompt files are VS Code prompt shims with YAML frontmatter and a markdown
    body. The MCP server does not synthesize prompt logic; it exposes those
    existing files so MCP clients with prompt catalogs can discover and request
    the same Bubbles entrypoints operators already use.
    """

    def __init__(self, prompts_dir: Path, logger: logging.Logger):
        self._prompts_dir = prompts_dir
        self._logger = logger
        self._prompts: dict[str, dict[str, Any]] = {}
        self._reload()

    @staticmethod
    def _split_prompt(text: str) -> tuple[dict[str, str], str]:
        if not text.startswith("---\n"):
            return {}, text.strip()
        end = text.find("\n---\n", 4)
        if end == -1:
            return {}, text.strip()
        frontmatter_text = text[4:end]
        body = text[end + len("\n---\n") :].strip()
        frontmatter: dict[str, str] = {}
        for raw_line in frontmatter_text.splitlines():
            line = raw_line.strip()
            if not line or line.startswith("#") or ":" not in line:
                continue
            key, value = line.split(":", 1)
            frontmatter[key.strip()] = value.strip().strip('"\'')
        return frontmatter, body

    def _reload(self) -> None:
        if not self._prompts_dir.is_dir():
            self._logger.info("prompt catalog empty: %s not found", self._prompts_dir)
            return
        for path in sorted(self._prompts_dir.glob("*.prompt.md")):
            text = path.read_text(encoding="utf-8")
            frontmatter, body = self._split_prompt(text)
            name = path.name.removesuffix(".prompt.md")
            description = frontmatter.get("description", "")
            agent = frontmatter.get("agent", name)
            self._prompts[name] = {
                "name": name,
                "description": description,
                "agent": agent,
                "body": body,
                "path": str(path),
            }
            self._logger.debug("loaded prompt: %s -> %s", name, path)

    def list_prompts(self) -> list[dict[str, Any]]:
        out: list[dict[str, Any]] = []
        for name, spec in self._prompts.items():
            out.append(
                {
                    "name": name,
                    "description": spec.get("description", ""),
                    "arguments": [],
                }
            )
        return out

    def get(self, name: str) -> Optional[dict[str, Any]]:
        return self._prompts.get(name)


# ---------------------------------------------------------------------------
# Tool execution
#
# Every tool invocation = subprocess against the bash twin. We capture
# stdout/stderr verbatim and surface the exit code so the calling agent
# sees the raw evidence — no summarization, no truncation by the server.

def _render_args(template: list[str], arguments: dict[str, Any]) -> list[str]:
    """Substitute `${var}` placeholders in argsTemplate with arguments[var].

    Syntax:
      ${var}     — required; raises ValueError if missing or null.
      ${var?}    — optional; if missing/empty/null, the ENTIRE argv element
                   containing the placeholder is DROPPED (not the placeholder
                   substituted with ''). This lets the catalog declare
                   `argsTemplate: ["${action}", "${gate_id?}"]` and produce
                   either `[json, G024]` or just `[list]` depending on input.

    Unknown placeholders (not `?`-suffixed) raise ValueError so the caller
    gets ERR_INVALID_PARAMS rather than passing the literal text down.
    """
    rendered: list[str] = []
    for tok in template:
        if "${" not in tok:
            rendered.append(tok)
            continue
        out = tok
        drop_this_arg = False
        i = 0
        while True:
            start = out.find("${", i)
            if start == -1:
                break
            end = out.find("}", start + 2)
            if end == -1:
                break
            raw = out[start + 2 : end]
            optional = raw.endswith("?")
            var = raw[:-1] if optional else raw
            if var not in arguments or arguments[var] is None or arguments[var] == "":
                if optional:
                    drop_this_arg = True
                    break
                raise ValueError(f"missing argument: {var}")
            value = arguments[var]
            out = out[:start] + str(value) + out[end + 1 :]
            i = start + len(str(value))
        if drop_this_arg:
            continue
        rendered.append(out)
    return rendered


def _execute_tool(
    spec: dict[str, Any],
    arguments: dict[str, Any],
    repo_root: Path,
    logger: logging.Logger,
) -> dict[str, Any]:
    """Run a tool's bash twin and return an MCP toolResult content array."""
    # Apply schema defaults: every property declared in inputSchema with a
    # `default` value is merged into arguments before rendering, unless the
    # caller already provided a value. This matches what most MCP clients
    # do client-side, but we apply it server-side too so tools work even
    # when invoked by a minimal client that doesn't honor defaults.
    schema = spec.get("inputSchema") or {}
    properties = schema.get("properties") or {}
    merged_args = dict(arguments)
    for prop, prop_spec in properties.items():
        if prop not in merged_args and "default" in prop_spec:
            merged_args[prop] = prop_spec["default"]
    try:
        rendered_args = _render_args(spec.get("argsTemplate", []), merged_args)
    except ValueError as exc:
        return {
            "content": [{"type": "text", "text": f"invalid arguments: {exc}"}],
            "isError": True,
        }

    cmd = ["bash", spec["_scriptAbsPath"], *rendered_args]
    timeout = int(
        os.environ.get(
            "BUBBLES_MCP_TOOL_TIMEOUT", spec.get("timeoutSeconds", DEFAULT_TOOL_TIMEOUT_SECONDS)
        )
    )
    started = time.time()
    logger.debug("tool exec: %s (timeout=%ds)", cmd, timeout)
    try:
        completed = subprocess.run(
            cmd,
            cwd=repo_root,
            capture_output=True,
            text=True,
            timeout=timeout,
            check=False,
        )
    except subprocess.TimeoutExpired as exc:
        return {
            "content": [
                {
                    "type": "text",
                    "text": (
                        f"tool '{spec['name']}' timed out after {timeout}s\n"
                        f"command: {' '.join(cmd)}\n"
                        f"partial stdout:\n{exc.stdout or ''}\n"
                        f"partial stderr:\n{exc.stderr or ''}"
                    ),
                }
            ],
            "isError": True,
        }
    elapsed = time.time() - started
    success_codes = spec.get("successExitCodes", [0])
    is_error = completed.returncode not in success_codes

    # Surface raw stdout + stderr + envelope. No summarization.
    envelope = {
        "command": cmd,
        "exitCode": completed.returncode,
        "durationSeconds": round(elapsed, 3),
        "tool": spec["name"],
    }
    content = [
        {
            "type": "text",
            "text": (
                f"$ {' '.join(cmd)}\n"
                f"exit={completed.returncode} duration={elapsed:.3f}s\n"
                f"--- stdout ---\n{completed.stdout}\n"
                f"--- stderr ---\n{completed.stderr}"
            ),
        }
    ]
    return {
        "content": content,
        "isError": is_error,
        "_meta": envelope,
    }


# ---------------------------------------------------------------------------
# JSON-RPC method dispatch

class Server:
    def __init__(
        self,
        transport: StdioTransport,
        tools: ToolCatalog,
        resources: ResourceCatalog,
        prompts: PromptCatalog,
        repo_root: Path,
        scripts_dir: Path,
        version: str,
        logger: logging.Logger,
    ):
        self._transport = transport
        self._tools = tools
        self._resources = resources
        self._prompts = prompts
        self._repo_root = repo_root
        self._scripts_dir = scripts_dir
        self._version = version
        self._logger = logger
        self._initialized = False

    def serve(self) -> int:
        while True:
            try:
                msg = self._transport.read_message()
            except ValueError as exc:
                self._logger.error("framing error: %s", exc)
                # JSON-RPC says we can't respond to a malformed message
                # without an id; log and try to recover by reading the next.
                continue
            except Exception as exc:
                self._logger.error("read error: %s", exc)
                return 1
            if msg is None:
                self._logger.info("stdin EOF; shutting down")
                return 0
            self._handle(msg)

    def _handle(self, msg: dict[str, Any]) -> None:
        response = self.handle_message(msg)
        if response is not None:
            self._transport.write_message(response)

    def handle_message(self, msg: dict[str, Any]) -> Optional[dict[str, Any]]:
        """Process a single JSON-RPC message and return the response dict.

        Returns None for notifications (no id) and for messages with no
        method. Transport-agnostic: used by BOTH the stdio loop and the HTTP
        transport (v6.1 / R9). The dispatch logic is identical across
        transports — only framing differs.
        """
        method = msg.get("method")
        msg_id = msg.get("id")
        params = msg.get("params") or {}
        if not method:
            self._logger.debug("ignoring response/notification with no method")
            return None

        # Notifications (no id) — never reply.
        if msg_id is None:
            self._handle_notification(method, params)
            return None

        # Requests — must reply.
        try:
            result = self._dispatch(method, params)
            return {"jsonrpc": "2.0", "id": msg_id, "result": result}
        except _JsonRpcError as exc:
            return self._error_obj(msg_id, exc.code, exc.message, exc.data)
        except Exception as exc:
            self._logger.exception("internal error handling %s", method)
            return self._error_obj(msg_id, ERR_INTERNAL, str(exc), None)

    def _handle_notification(self, method: str, params: dict[str, Any]) -> None:
        # We accept "notifications/initialized" as a no-op handshake completion.
        # Everything else is logged and ignored.
        if method == "notifications/initialized":
            self._initialized = True
            self._logger.info("client signaled initialized")
            return
        self._logger.debug("ignoring notification: %s", method)

    def _dispatch(self, method: str, params: dict[str, Any]) -> Any:
        if method == "initialize":
            return self._initialize(params)
        if method == "ping":
            return {}
        if method == "tools/list":
            return {"tools": self._tools.list_tools()}
        if method == "tools/call":
            return self._tools_call(params)
        if method == "resources/list":
            return {"resources": self._resources.list_resources()}
        if method == "resources/templates/list":
            return {"resourceTemplates": self._resources.list_resource_templates()}
        if method == "resources/read":
            return self._resources_read(params)
        if method == "prompts/list":
            return {"prompts": self._prompts.list_prompts()}
        if method == "prompts/get":
            return self._prompts_get(params)
        raise _JsonRpcError(ERR_METHOD_NOT_FOUND, f"unknown method: {method}")

    def _initialize(self, params: dict[str, Any]) -> dict[str, Any]:
        # Protocol version negotiation (per MCP spec): echo the client's
        # requested version when we support it; otherwise return our latest
        # supported version and let the client decide whether to proceed.
        requested = params.get("protocolVersion")
        if isinstance(requested, str) and requested in SUPPORTED_PROTOCOL_VERSIONS:
            negotiated = requested
        else:
            negotiated = PROTOCOL_VERSION
        return {
            "protocolVersion": negotiated,
            "serverInfo": {
                "name": SERVER_NAME,
                "version": self._version,
            },
            "capabilities": {
                "tools": {"listChanged": False},
                "resources": {
                    "listChanged": False,
                    "subscribe": False,
                },
                "prompts": {"listChanged": False},
            },
            "instructions": (
                "Bubbles MCP server. Tools are thin wrappers around "
                "bubbles/scripts/*.sh — every call records the actual command "
                "line and full stdout/stderr in the response. Resources are "
                "read-only handles to canonical Bubbles files; templated "
                "resources (bubbles://gates/{id}, bubbles://spec/{nnn}/state.json) "
                "expand per-id/per-spec. Prompts expose the repo's existing "
                "Bubbles prompt shims. No summarization."
            ),
        }

    def _tools_call(self, params: dict[str, Any]) -> dict[str, Any]:
        name = params.get("name")
        if not name:
            raise _JsonRpcError(ERR_INVALID_PARAMS, "missing 'name'")
        arguments = params.get("arguments") or {}
        spec = self._tools.get(name)
        if spec is None:
            raise _JsonRpcError(ERR_TOOL_NOT_FOUND, f"unknown tool: {name}")
        return _execute_tool(spec, arguments, self._repo_root, self._logger)

    def _resources_read(self, params: dict[str, Any]) -> dict[str, Any]:
        uri = params.get("uri")
        if not uri:
            raise _JsonRpcError(ERR_INVALID_PARAMS, "missing 'uri'")
        result = self._resources.read(uri)
        if result is None:
            raise _JsonRpcError(ERR_RESOURCE_NOT_FOUND, f"unknown resource: {uri}")
        if "error" in result:
            raise _JsonRpcError(ERR_RESOURCE_FAILED, result["error"])
        return {
            "contents": [
                {
                    "uri": uri,
                    "mimeType": result.get("mimeType", "text/plain"),
                    "text": result["text"],
                }
            ]
        }

    def _prompts_get(self, params: dict[str, Any]) -> dict[str, Any]:
        name = params.get("name")
        if not name:
            raise _JsonRpcError(ERR_INVALID_PARAMS, "missing 'name'")
        spec = self._prompts.get(name)
        if spec is None:
            raise _JsonRpcError(ERR_PROMPT_NOT_FOUND, f"unknown prompt: {name}")
        body = spec.get("body", "")
        description = spec.get("description", "")
        agent = spec.get("agent", name)
        text = body
        if agent:
            text = f"Use agent: {agent}\n\n{text}".strip()
        return {
            "description": description,
            "messages": [
                {
                    "role": "user",
                    "content": {
                        "type": "text",
                        "text": text,
                    },
                }
            ],
        }

    def _reply_result(self, msg_id: Any, result: Any) -> None:
        self._transport.write_message(
            {"jsonrpc": "2.0", "id": msg_id, "result": result}
        )

    def _error_obj(
        self,
        msg_id: Any,
        code: int,
        message: str,
        data: Any = None,
    ) -> dict[str, Any]:
        err: dict[str, Any] = {"code": code, "message": message}
        if data is not None:
            err["data"] = data
        return {"jsonrpc": "2.0", "id": msg_id, "error": err}

    def _reply_error(
        self,
        msg_id: Any,
        code: int,
        message: str,
        data: Any = None,
    ) -> None:
        self._transport.write_message(
            self._error_obj(msg_id, code, message, data)
        )


class _JsonRpcError(Exception):
    def __init__(self, code: int, message: str, data: Any = None):
        super().__init__(message)
        self.code = code
        self.message = message
        self.data = data


# ---------------------------------------------------------------------------
# HTTP transport (v6.1 / R9) — stdlib only. Makes the gate surface reachable
# from CI runners and shared/cloud environments, not just a local stdio shell.
# Same JSON-RPC dispatch as stdio (Server.handle_message); only framing differs.

def serve_http(server: "Server", host: str, port: int,
               logger: logging.Logger) -> int:
    """Serve the MCP JSON-RPC surface over HTTP POST.

    Single endpoint: POST / (or POST /rpc) with a JSON-RPC request body;
    responds with the JSON-RPC response body. GET /health returns 200 for
    liveness probes. Optional bearer-token auth via BUBBLES_MCP_HTTP_TOKEN.
    """
    import http.server

    auth_token = os.environ.get("BUBBLES_MCP_HTTP_TOKEN", "").strip()

    class _Handler(http.server.BaseHTTPRequestHandler):
        # Quiet the default stderr access log; route through our file logger.
        def log_message(self, fmt: str, *args: Any) -> None:
            logger.debug("http %s - " + fmt, self.address_string(), *args)

        def _authorized(self) -> bool:
            if not auth_token:
                return True
            header = self.headers.get("Authorization", "")
            expected = f"Bearer {auth_token}"
            return header == expected

        def _send_json(self, status: int, obj: Any) -> None:
            payload = json.dumps(obj).encode("utf-8")
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)

        def do_GET(self) -> None:  # noqa: N802 (stdlib naming)
            if self.path.rstrip("/") in ("/health", "/healthz"):
                self._send_json(200, {"status": "ok", "server": SERVER_NAME})
                return
            self._send_json(404, {"error": "not found"})

        def do_POST(self) -> None:  # noqa: N802 (stdlib naming)
            if not self._authorized():
                self._send_json(401, {"jsonrpc": "2.0", "id": None,
                                      "error": {"code": ERR_INVALID_REQUEST,
                                                "message": "unauthorized"}})
                return
            try:
                length = int(self.headers.get("Content-Length", "0"))
            except ValueError:
                length = 0
            raw = self.rfile.read(length) if length > 0 else b""
            try:
                msg = json.loads(raw.decode("utf-8")) if raw else {}
            except (ValueError, UnicodeDecodeError) as exc:
                self._send_json(200, {"jsonrpc": "2.0", "id": None,
                                      "error": {"code": ERR_PARSE,
                                                "message": f"parse error: {exc}"}})
                return
            response = server.handle_message(msg)
            if response is None:
                self.send_response(204)
                self.end_headers()
                return
            self._send_json(200, response)

    httpd = http.server.ThreadingHTTPServer((host, port), _Handler)
    logger.info("bubbles-mcp HTTP transport listening on %s:%d (auth=%s)",
                host, port, "on" if auth_token else "off")
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        logger.info("HTTP transport interrupted; shutting down")
    finally:
        httpd.server_close()
    return 0


# ---------------------------------------------------------------------------
# Entry point

def _configure_logging() -> logging.Logger:
    level_name = os.environ.get("BUBBLES_MCP_LOG_LEVEL", "INFO").upper()
    level = getattr(logging, level_name, logging.INFO)
    log_file = os.environ.get(
        "BUBBLES_MCP_LOG_FILE",
        os.path.join(os.environ.get("TMPDIR", "/tmp"), "bubbles-mcp-server.log"),
    )
    logger = logging.getLogger("bubbles-mcp")
    logger.setLevel(level)
    # NEVER log to stderr in stdio transport mode; MCP clients can swallow
    # or surface stderr inconsistently. File only.
    try:
        handler = logging.FileHandler(log_file, mode="a", encoding="utf-8")
    except OSError:
        # If we can't open the log file, fall back to a no-op handler. The
        # server still works; we just lose diagnostics.
        handler = logging.NullHandler()
    handler.setFormatter(
        logging.Formatter("%(asctime)s %(levelname)s %(name)s: %(message)s")
    )
    logger.addHandler(handler)
    return logger


def _validate_runtime() -> None:
    if not shutil.which("bash"):
        sys.stderr.write("bubbles-mcp: bash not found in PATH\n")
        sys.exit(1)


def main() -> int:
    _validate_runtime()
    # Minimal CLI parsing (stdlib only; no argparse dependency on the hot path).
    transport_kind = "stdio"
    http_host = os.environ.get("BUBBLES_MCP_HTTP_HOST", "127.0.0.1")
    http_port = int(os.environ.get("BUBBLES_MCP_HTTP_PORT", "8765"))
    argv = sys.argv[1:]
    i = 0
    while i < len(argv):
        arg = argv[i]
        if arg == "--transport":
            i += 1
            transport_kind = argv[i] if i < len(argv) else "stdio"
        elif arg.startswith("--transport="):
            transport_kind = arg.split("=", 1)[1]
        elif arg == "--host":
            i += 1
            http_host = argv[i] if i < len(argv) else http_host
        elif arg.startswith("--host="):
            http_host = arg.split("=", 1)[1]
        elif arg == "--port":
            i += 1
            http_port = int(argv[i]) if i < len(argv) else http_port
        elif arg.startswith("--port="):
            http_port = int(arg.split("=", 1)[1])
        elif arg in ("-h", "--help"):
            sys.stderr.write(
                "Usage: server.py [--transport stdio|http] [--host H] [--port P]\n"
            )
            return 0
        else:
            sys.stderr.write(f"bubbles-mcp: unknown argument: {arg}\n")
            return 2
        i += 1
    if transport_kind not in ("stdio", "http"):
        sys.stderr.write(f"bubbles-mcp: unknown transport: {transport_kind}\n")
        return 2

    logger = _configure_logging()
    repo_root = _detect_repo_root()
    scripts_dir = _resolve_scripts_dir(repo_root)
    mcp_dir = _resolve_mcp_dir(repo_root)
    prompts_dir = _resolve_prompts_dir(repo_root)
    version = _resolve_server_version(repo_root)
    logger.info(
        "starting bubbles-mcp v%s transport=%s repo_root=%s scripts=%s mcp=%s prompts=%s",
        version,
        transport_kind,
        repo_root,
        scripts_dir,
        mcp_dir,
        prompts_dir,
    )
    tools = ToolCatalog(mcp_dir, scripts_dir, logger)
    resources = ResourceCatalog(mcp_dir, repo_root, scripts_dir, logger)
    prompts = PromptCatalog(prompts_dir, logger)
    transport = StdioTransport(sys.stdin.buffer, sys.stdout.buffer)
    server = Server(
        transport, tools, resources, prompts, repo_root, scripts_dir, version, logger
    )
    if transport_kind == "http":
        return serve_http(server, http_host, http_port, logger)
    return server.serve()


if __name__ == "__main__":
    sys.exit(main())
