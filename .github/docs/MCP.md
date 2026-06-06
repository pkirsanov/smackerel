# Bubbles MCP Server (v6.0)

> Status: SHIPPED in v6.0. Optional — bash scripts remain the supported fallback.
> Transport: stdio (default) or HTTP (v6.1 / R9). Protocol: Model Context Protocol (MCP) 2024-11-05.
> Runtime: Python 3.10+, stdlib only. No `pip install`. No daemon.

The Bubbles MCP server exposes the framework's gate registry, validation scripts, and canonical resources as MCP tools and resources so MCP-aware clients (VS Code Copilot Chat agent, Claude Desktop, Cursor, Cline) can call them directly — without spawning shell processes or parsing markdown.

Every tool is a thin wrapper around an existing `bubbles/scripts/*.sh`. The server NEVER duplicates business logic. Every tool response surfaces the bash twin's stdout, stderr, exit code, and command line verbatim — no summarization, no truncation, no paraphrasing.

If you do not register the MCP server, every Bubbles workflow continues to work unchanged via the bash scripts.

---

## Quick Start

### 1. Verify the server boots

```bash
bash .github/bubbles/scripts/mcp-server-selftest.sh
# Expected: 11 PASS lines and "mcp-server-selftest passed."
```

### 2. Register with your MCP client

Sample configs ship under `.github/bubbles/mcp/clients/`. Pick the one for your client and merge the `mcpServers.bubbles` block into your client's config file:

| Client | Config file | Sample |
|--------|-------------|--------|
| VS Code (Copilot Chat agent) | `.vscode/settings.json` or workspace settings | `.github/bubbles/mcp/clients/vscode.json` |
| Claude Desktop | `~/.config/Claude/claude_desktop_config.json` | `.github/bubbles/mcp/clients/claude.json` |
| Cursor | `.cursor/mcp.json` (workspace) or `~/.cursor/mcp.json` (global) | `.github/bubbles/mcp/clients/cursor.json` |
| Cline | `cline_mcp_settings.json` | `.github/bubbles/mcp/clients/cline.json` |

Restart your client. The `bubbles` server should appear with 10 tools and 5 resources.

---

## Tool Catalog

| Tool | Bash twin | Purpose |
|------|-----------|---------|
| `validate_dod` | `state-transition-guard.sh` | Validate a spec's DoD evidence, scope status, and transition gates. Use before flipping `state.json` status to `done`. |
| `verify_status_transition` | `state-transition-guard.sh` | Alias of `validate_dod` framed for status transitions. |
| `record_evidence` | `tool-log.sh` | Wrap a shell command and append a structured tool-call entry to `.specify/runtime/tool-calls.jsonl`. Primary v5.1+ evidence path. |
| `check_gate` | `gate-meta.sh` | Look up gate metadata from `bubbles/registry/gates.yaml` (json/name/description/exists/list/count). |
| `resolve_mode` | `mode-resolver.sh` | Resolve a workflow mode name to its expanded YAML definition. |
| `route_finding` | `discovered-issue-disposition-guard.sh` | Classify a discovered finding (in-scope / defer / sibling-spec). |
| `query_tool_log` | `evidence-tool-log-bridge.sh` | Query the tool-call log for DoD-item coverage. |
| `search_code` | `code-search.sh` | Pattern search with auto-selected backend (ripgrep/grep). |
| `read_spec` | `artifact-lint.sh` | Inventory + lint a spec directory's artifacts. |
| `list_open_findings` | `finding-closure-selftest.sh` | Surface the active finding-closure contract. |

Tool definitions live in `.github/bubbles/mcp/tools/*.json`. Each declares an `inputSchema` (JSON Schema) and an `argsTemplate` with `${var}` (required) and `${var?}` (optional, drop-on-empty) placeholders.

---

## Resource Catalog

| URI | File | MIME |
|-----|------|------|
| `bubbles://workflows.yaml` | `bubbles/workflows.yaml` | `application/yaml` |
| `bubbles://registry/gates.yaml` | `bubbles/registry/gates.yaml` | `application/yaml` |
| `bubbles://agent-capabilities.yaml` | `bubbles/agent-capabilities.yaml` | `application/yaml` |
| `bubbles://schemas/tool-call` | `bubbles/schemas/tool-call.schema.json` | `application/json` |
| `bubbles://workflows/intent-routes.yaml` | `bubbles/intent-routes.yaml` | `application/yaml` |

Resources are read-only. Templated URIs (`bubbles://gates/{id}`, `bubbles://spec/{nnn}/state.json`) are deferred to v6.1.

---

## Environment Variables

| Variable | Default | Effect |
|----------|---------|--------|
| `BUBBLES_MCP_REPO_ROOT` | auto-detected | Override repo-root walk-up. |
| `BUBBLES_MCP_TOOL_TIMEOUT` | 300 (seconds) | Hard cap per tool invocation. |
| `BUBBLES_MCP_LOG_LEVEL` | `INFO` | `DEBUG\|INFO\|WARNING\|ERROR`. |
| `BUBBLES_MCP_LOG_FILE` | `$TMPDIR/bubbles-mcp-server.log` | Diagnostic log path. |

The server NEVER writes diagnostics to stderr during stdio operation — only to the log file — because MCP clients handle stderr inconsistently.

---

## Anti-Fabrication Guarantees

The server enforces the same anti-fabrication discipline as the bash scripts:

1. **Every tool call records the actual command.** The response `_meta.command` is the exact `argv` array passed to `subprocess.run`.
2. **No summarization.** The response `content[0].text` is `$ <command>\nexit=N duration=Xs\n--- stdout ---\n<stdout>\n--- stderr ---\n<stderr>`. The server never paraphrases.
3. **Every tool has a bash twin.** The catalog loader REJECTS any tool spec that points to a missing script (fail fast at startup).
4. **Bash fallback always works.** Repos that don't register MCP still run every gate via the bash scripts. MCP is additive UX, not a replacement runtime.

---

## Troubleshooting

| Symptom | Diagnosis |
|---------|-----------|
| Server doesn't appear in client | Check client logs for "failed to spawn"; verify `python3` is on PATH; check the path in the client config resolves to `server.py`. |
| Selftest passes but client shows zero tools | Client may not be honoring the `initialize` capability response. Check client version. |
| Tool call returns "missing argument" | The bash twin requires a positional argument that the caller did not supply. Check the tool's `inputSchema.required`. |
| Tool call hangs | The bash twin is doing real work; default timeout is 300s. Raise `BUBBLES_MCP_TOOL_TIMEOUT` or the tool spec's `timeoutSeconds`. |
| Resource read returns `-32004` | The file declared in the resource catalog doesn't exist. The catalog loader doesn't pre-validate resource paths — only tool scripts. |

---

## Selftest

`bash .github/bubbles/scripts/mcp-server-selftest.sh` asserts 11 invariants (T1–T11): server boots, every declared tool has a bash twin, `initialize`/`ping`/`tools/list`/`tools/call`/`resources/list`/`resources/read` round-trip correctly, malformed/unknown requests return proper JSON-RPC error codes, optional `${var?}` substitution works.

The selftest is wired into `bubbles/scripts/framework-validate.sh` so the v6 MCP invariant is enforced on every source-side framework-validate run.

## HTTP Transport (v6.1 / R9)

The server also speaks JSON-RPC over HTTP so the gate surface is reachable from CI runners and shared/cloud environments, not just a local stdio shell:

```bash
python3 .github/bubbles/mcp/server.py --transport http --host 127.0.0.1 --port 8765
```

- `POST /` (or `/rpc`) with a JSON-RPC request body returns the JSON-RPC response. Notifications return `204`.
- `GET /health` returns `200 {"status":"ok"}` for liveness probes.
- Optional bearer auth: set `BUBBLES_MCP_HTTP_TOKEN=<token>` and send `Authorization: Bearer <token>`; missing/invalid tokens get `401`.
- Host/port also configurable via `BUBBLES_MCP_HTTP_HOST` / `BUBBLES_MCP_HTTP_PORT`.
- Same JSON-RPC dispatch as stdio (`Server.handle_message`); only framing differs, so every tool/resource behaves identically across transports.
- Validated by `bubbles/scripts/mcp-http-transport-selftest.sh` (boots on an ephemeral port; asserts initialize/tools-list round-trips, health, bearer auth, and parse-error handling), wired into `framework-validate`.

---

## What v6.0 Does Not Do

- HTTP/SSE transport (HTTP POST shipped in v6.1 / R9; Server-Sent Events streaming still deferred).
- Templated resource URIs like `bubbles://gates/{id}` (deferred to v6.1).
- Server-initiated notifications (`resources/listChanged`, `tools/listChanged` events).
- Auth / per-tool authorization (the server inherits the OS user's permissions).
- Server installation via `pip install` or `npm install` — the design is intentionally dependency-free.
