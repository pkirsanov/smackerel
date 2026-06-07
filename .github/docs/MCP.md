# Bubbles MCP Server

> Status: SHIPPED (since v6.0). Optional — bash scripts remain the supported fallback.
> Transport: stdio (default) or HTTP. Protocol: Model Context Protocol (MCP) — negotiates 2024-11-05 / 2025-03-26 / 2025-06-18 (echoes the client's requested version when supported, else returns the latest).
> Runtime: Python 3.10+, stdlib only. No `pip install`. No daemon.

The Bubbles MCP server exposes the framework's gate registry, validation scripts, and canonical resources as MCP tools and resources so MCP-aware clients (VS Code Copilot Chat agent, Claude Desktop, Cursor, Cline) can call them directly — without spawning shell processes or parsing markdown.

Every tool is a thin wrapper around an existing `bubbles/scripts/*.sh`. The server NEVER duplicates business logic. Every tool response surfaces the bash twin's stdout, stderr, exit code, and command line verbatim — no summarization, no truncation, no paraphrasing.

If you do not register the MCP server, every Bubbles workflow continues to work unchanged via the bash scripts.

---

## Quick Start

### 1. Verify the server boots

```bash
bash .github/bubbles/scripts/mcp-server-selftest.sh
# Expected: T1–T19 PASS lines and "mcp-server-selftest passed."
```

### 2. Register with your MCP client

Sample configs ship under `.github/bubbles/mcp/clients/`. Pick the one for your client and merge the `mcpServers.bubbles` block into your client's config file:

| Client | Config file | Sample |
|--------|-------------|--------|
| VS Code (Copilot Chat agent) | `.vscode/settings.json` or workspace settings | `.github/bubbles/mcp/clients/vscode.json` |
| Claude Desktop | `~/.config/Claude/claude_desktop_config.json` | `.github/bubbles/mcp/clients/claude.json` |
| Cursor | `.cursor/mcp.json` (workspace) or `~/.cursor/mcp.json` (global) | `.github/bubbles/mcp/clients/cursor.json` |
| Cline | `cline_mcp_settings.json` | `.github/bubbles/mcp/clients/cline.json` |

Restart your client. The `bubbles` server should appear with 10 annotated tools, 5 static resources, 2 resource templates, and 37 prompts.

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

Tools also expose MCP annotations so newer clients can plan safely:

- read-only query tools (`check_gate`, `resolve_mode`, `read_spec`, `search_code`, validation/readback tools) advertise `readOnlyHint: true`, `destructiveHint: false`, `idempotentHint: true`, and `openWorldHint: false`.
- `record_evidence` advertises `readOnlyHint: false`, `destructiveHint: true`, `idempotentHint: false`, and `openWorldHint: true` because it wraps an arbitrary command and appends to the tool-call log.

---

## Resource Catalog

| URI | File | MIME |
|-----|------|------|
| `bubbles://workflows.yaml` | `bubbles/workflows.yaml` | `application/yaml` |
| `bubbles://registry/gates.yaml` | `bubbles/registry/gates.yaml` | `application/yaml` |
| `bubbles://agent-capabilities.yaml` | `bubbles/agent-capabilities.yaml` | `application/yaml` |
| `bubbles://schemas/tool-call` | `bubbles/schemas/tool-call.schema.json` | `application/json` |
| `bubbles://workflows/intent-routes.yaml` | `bubbles/intent-routes.yaml` | `application/yaml` |

Resources are read-only.

### Resource Templates

The server also exposes **templated resources** (RFC 6570 level-1 `{var}`
expansion), discoverable via the `resources/templates/list` method:

| URI template | Resolves to | MIME |
|--------------|-------------|------|
| `bubbles://gates/{id}` | One gate's full metadata, e.g. `bubbles://gates/G024`. Backed by the `gate-meta.sh json <id>` bash twin — the same source the `check_gate` tool uses, never a duplicated parser. | `application/json` |
| `bubbles://spec/{nnn}/state.json` | A spec's control-plane `state.json`, e.g. `bubbles://spec/042/state.json` → `specs/042*/state.json`. Resolves only in downstream consumer repos (the Bubbles source repo keeps no `specs/` per the G085 dogfood guard). | `application/json` |

Templated reads surface the same anti-fabrication guarantee as tools: a
`commandTemplate`-backed resource (like `bubbles://gates/{id}`) runs its bash
twin and returns the verbatim stdout; an unknown id or an unmatched/ambiguous
spec number returns a real `-32004` (`ERR_RESOURCE_FAILED`) error, never a fake
empty success. Extracted `{var}` values may not contain `..`.

---

## Prompt Catalog

The MCP server exposes the same prompt shims Bubbles installs for VS Code and other agent clients:

| MCP method | Backing files | Result |
|------------|---------------|--------|
| `prompts/list` | `prompts/*.prompt.md` in the source repo, `.github/prompts/*.prompt.md` downstream | Lists every Bubbles prompt shim by name and description. |
| `prompts/get` | One selected `.prompt.md` file | Returns a single user message containing the prompt body plus the target `agent:` from frontmatter. |

Prompt exposure is read-only and does not synthesize new prompt logic. The server parses the existing frontmatter (`agent`, `description`) and body, so MCP clients that surface prompt catalogs can invoke the same Bubbles entrypoints as slash-prompt users. Unknown prompt names return a real `-32005` (`ERR_PROMPT_NOT_FOUND`) error.

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

`bash .github/bubbles/scripts/mcp-server-selftest.sh` asserts 19 invariants (T1–T19): server boots, every declared tool has a bash twin, `initialize`/`ping`/`tools/list`/`tools/call`/`resources/list`/`resources/read` round-trip correctly, `resources/templates/list` returns the templated catalog, templated reads (`bubbles://gates/{id}`) resolve via the bash twin and surface real `-32004` errors for unknown ids, `prompts/list` returns the prompt catalog, `prompts/get` returns a real prompt body, unknown prompts return `-32005`, `tools/list` exposes planning/safety annotations, `initialize` negotiates the protocol version (echo-when-supported, latest-otherwise), malformed/unknown requests return proper JSON-RPC error codes, and optional `${var?}` substitution works.

The selftest is wired into `bubbles/scripts/framework-validate.sh` so the MCP invariant is enforced on every source-side framework-validate run.

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

## What The Server Does Not Do

- Server-Sent Events (SSE) streaming over the HTTP transport (HTTP POST + health only).
- Server-initiated notifications (`resources/listChanged`, `tools/listChanged` events).
- Prompt argument templating — current Bubbles prompt shims are static and accept no MCP prompt arguments.
- Auth / per-tool authorization beyond the optional HTTP bearer token (the server otherwise inherits the OS user's permissions).
- Server installation via `pip install` or `npm install` — the design is intentionally dependency-free.
