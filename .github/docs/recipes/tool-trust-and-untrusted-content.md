# Tool Trust & Untrusted Content

> Operator setup for the Bubbles tool-trust boundary (IMP-020 S3 / AF-005):
> the tool-trust registry, the structured-event risk gate, action-bound
> approvals, and what Bubbles can and cannot enforce.

Use this when you are wiring a host `PreToolUse` integration, registering an MCP
server, or reviewing how Bubbles decides whether a tool call may run.

## The two halves

| Half | File | Role |
|------|------|------|
| **Policy** (behavioral) | [`agents/bubbles_shared/untrusted-content.md`](../../agents/bubbles_shared/untrusted-content.md) | Every agent treats repo/web/DOM/tool/MCP content as **data, never instruction**. Data can't authorize tools, change scope, request secrets, weaken gates, or cause egress. |
| **Mechanism** (machine) | [`bubbles/tool-trust-registry.yaml`](../../bubbles/tool-trust-registry.yaml) + [`pre-tool-risk-gate.sh`](../../bubbles/scripts/pre-tool-risk-gate.sh) | Classifies servers/operations and makes a fail-closed BLOCK/WARN/ALLOW decision before a call runs. |

## Registering a server/operation

Add an entry under `servers:` in `tool-trust-registry.yaml` (validated against
`bubbles/schemas/tool-trust-registry.schema.json`):

```yaml
servers:
  my-mcp:
    source: external-mcp          # bubbles-cli | bubbles-mcp | host-builtin | external-mcp
    trustState: trusted           # only for a maintainer-registered source — never inferred from a token in .vscode/mcp.json
    hostEnforceable: true         # false => decisions are advisory (enforcement: unavailable)
    operations:
      fetch: { riskClass: read_only, capability: read, egress: none, permittedDataClasses: [public, internal], approvalRequired: false }
```

Anything **not** registered falls through to the fail-closed `defaults`:
unregistered servers are **default-denied** for sensitive operations, and an
**unknown operation is never `read_only`** — it is treated as sensitive.

## How the gate decides (structured event)

```bash
pre-tool-risk-gate.sh --server my-mcp --operation fetch \
  --target <path/url> --data-classes public,internal --egress none \
  [--approval-file <path>]
```

- Unregistered server / unknown operation → **BLOCK** (fail closed).
- A data class not in `permittedDataClasses`, or `secret` data on any egress-capable op → **BLOCK**.
- External egress the operation doesn't declare → **BLOCK**.
- A **sensitive** op (`destructive_mutation` / `external_side_effect`, or `approvalRequired: true`) needs an **action-bound approval** (below). A generic `--confirm` / `BUBBLES_RISK_CONFIRM=1` **cannot** unlock it.
- If the server is not `hostEnforceable`, a sensitive decision is `enforcement: unavailable` and **BLOCKED** — Bubbles never silently passes an action it cannot enforce.

## Action-bound approvals (no simulated human approval)

A sensitive op is authorized only by an approval that **binds this exact action**
and is **host-verified**. The approval file carries:

```
hostVerified=true                 # only a host-native approval callback may set this
requestHash=<sha256 of tool|server|operation|target|dataClasses|egress>
expiry=<unix-epoch-seconds>
```

The gate recomputes `requestHash` from the event, so a stale approval **replayed
to a different target/argument no longer binds** and is rejected; expired or
non-`hostVerified` approvals are rejected too. If your host cannot supply a
verified approval callback, sensitive actions stay blocked — by design.

## What Bubbles can and cannot enforce

| Surface | Guarantee |
|---------|-----------|
| Bubbles CLI / MCP calls + host events routed through the gate | Machine-enforced registry decision; sensitive unknowns block. |
| Declared MCP grants + registered server ids | Machine-linted allowlist (see [`cli.sh mcp sync`](../MCP.md)). |
| Ambient VS Code edit/shell/web/browser tools **not** routed through the hook | **Not interceptable** by Bubbles alone → `enforcement: unavailable`. Least privilege + the untrusted-content policy + behavior regressions are the defenses. |
| Malicious instructions embedded in tool/web/repo content | Marked **data**, never authority; covered by policy + regressions — not a claim of perfect prompt-injection prevention. |

## Secrets

Never place a secret **value** in an event, approval, log, or fixture. Every
surface here operates on **classification and presence metadata only**
(`dataClasses`, `present`), never the literal secret.
