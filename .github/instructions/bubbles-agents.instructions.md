---
applyTo: "**"
---

# Agent File Guidelines

> Adapted from [github/awesome-copilot](https://github.com/github/awesome-copilot) (MIT License).
> Adapted for repo-local conventions. Keep this document project-agnostic.
>
> **Portability:** This file is **project-agnostic**. Copy unchanged across projects.
> See [project-config-contract.md](../agents/bubbles_shared/project-config-contract.md) for the cross-project portability rules.
> See [agent-common.md](../agents/bubbles_shared/agent-common.md) for universal governance (anti-fabrication, evidence standards, sequential completion).

## Agent File Format

Agent files live in `agents/` in the Bubbles source repo and `.github/agents/` in downstream installs. They use the `.agent.md` extension.

### Required Frontmatter

```yaml
---
description: <one-sentence summary of what the agent does>
---
```

### Optional Frontmatter Fields

```yaml
---
description: <required>
tools:
  - <tool-name>
  - <another-tool>
model: <model-preference>  # e.g., claude-sonnet-4-20250514, gpt-4.1
---
```

### Dispatch-Control Frontmatter (host-runtime enforced)

These fields are read by the VS Code runtime, not by the model. A delegation rule
expressible here MUST be expressed here — body prose is advisory, frontmatter is
enforced. Enforced by `bubbles/scripts/workflow-runner-grants-lint.sh` (Gate G064).

| Field | Meaning | Default |
|-------|---------|---------|
| `agents:` | Subagent allowlist. `*` = all, `[]` = none. **An explicit listing OVERRIDES a target's `disable-model-invocation`.** | `*` |
| `disable-model-invocation:` | Prevents this agent from being dispatched as a subagent. | `false` |
| `user-invocable:` | Whether the agent appears in the chat agents dropdown. | `true` |
| `handoffs:` | Buttons shown **after the turn ends**; the user clicks to **switch** agents. NOT subagent dispatch. | none |
| `handoffs[].send` | Auto-submits the handoff prompt. Bubbles requires this stay unset. | `false` |

Rules for `bubbles.*` agents:

- **Pure top-level runners** (granted a workflow mode but never a declared `owner:` phase)
  MUST set `disable-model-invocation: true`.
- **Dual-role agents** (granted runner AND declared phase owner) MUST NOT set it — doing so
  breaks the phase dispatch they exist to serve.
- No agent may name a pure top-level runner in `agents:`; that would override the flag.
- Never set `handoffs[].send: true`.
- Dispatch requires the `agent` tool alias in `tools:`; a body allowlist alone does nothing.

Full authoring guidance, the depth-1 subagent constraint, and a design-review checklist
live in the [`bubbles-vscode-agent-constraints`](../skills/bubbles-vscode-agent-constraints/SKILL.md)
skill; the role split and precedence rationale live in
[`workflow-delegation-core.md`](../agents/bubbles_shared/workflow-delegation-core.md).

## Prompt Shim Pattern

This repo uses a **prompt shim + agent definition** architecture:

1. **Prompt shims** (`prompts/*.prompt.md` in source, `.github/prompts/*.prompt.md` downstream): Minimal files with frontmatter routing to agents
2. **Agent definitions** (`agents/*.agent.md` in source, `.github/agents/*.agent.md` downstream): Full behavior and instructions

### Prompt Shim Example

```yaml
---
mode: agent
agent: my-agent
description: Brief description shown in UI
---
```

The shim contains no behavior—all logic lives in the agent file.

## Agent Body Structure

After frontmatter, use Markdown with clear sections:

```markdown
## Agent Identity
- **Name**: `<agent-id>` (must match filename and prompt shim)
- **Role**: One sentence describing what the agent is responsible for.
- **Non-goals**: What this agent explicitly does not do.

## Purpose
Brief statement of what this agent does.

## Workflow
1. Step one
2. Step two
3. ...

## Rules
- Constraint or policy
- Another constraint

## Output Format
Description of expected output structure.
```

### Structural YAML Body Pattern For Orchestrators

Router/orchestrator agents such as `bubbles.workflow`, `bubbles.goal`, `bubbles.sprint`, and `bubbles.iterate` may use structural YAML blocks near the top of the body when deterministic routing is more important than narrative prose.

Required shape:

````markdown
## TOOL ALLOWLIST (ENFORCED)

```yaml
allowed_tools:
  always:
    - read_file
    - search_subagent
    - runSubagent
forbidden_tools:
  - runTests
```

## PHASE ROUTER (EXECUTE TOP-TO-BOTTOM)

```yaml
phase_1_understand:
  do: classify input and load minimal context
  call_runSubagent: no
phase_2_dispatch:
  do: route to the owning specialist or execute the mapped mode in the active authorized runner
  call_runSubagent: yes
```
````

Rules:
- Frontmatter remains authoritative for runtime tool availability. If the body allowlist names `runSubagent`, frontmatter MUST include the VS Code `agent` tool alias.
- Body-level allowlists are governance contracts, not a replacement for frontmatter `tools`.
- Structural YAML must declare phase order, delegation targets, stop conditions, and forbidden direct work clearly enough that downstream reviewers can distinguish routing from implementation.
- Keep prose outside the structural blocks short and reserved for identity, policy references, and result-envelope expectations.
- Do not use structural YAML to hide missing ownership, missing evidence, or implementation work performed by an orchestrator.

### Writing Effective Rules (Negative Rules First)

**Explicit prohibitions yield the biggest quality improvements.** When defining agent rules, prioritize what the agent must NEVER do over what it should do. The AI can often infer positive patterns from code context, but it cannot infer architectural boundaries, security constraints, or project-specific prohibitions.

| Rule Type | Effectiveness | Example |
|-----------|--------------|----------|
| **Prohibition** ("NEVER", "FORBIDDEN") | Highest impact | "NEVER use `unwrap()` in production code" |
| **Boundary** ("ONLY", "MUST NOT cross") | High impact | "Python services MUST NOT access databases directly" |
| **Constraint** ("MUST", "REQUIRED") | Medium impact | "All operations MUST have explicit timeouts" |
| **Preference** ("Prefer", "Use") | Lower impact | "Prefer borrowing over cloning" |

Focus instruction content on rules the AI cannot infer from the code alone — security guardrails, cross-layer restrictions, naming conventions, and architectural boundaries.

## Agent Definition Best Practices

- Start with **Agent Identity** so readers immediately know who/what is acting.
- Prefer a **phase-based workflow** (similar to `bubbles.iterate` or `bubbles.workflow`) when the task is multi-step:
  - Context loading → plan/selection → implement → tests/validation → docs → finalize
- Keep required inputs explicit:
  - Define what is **Required** vs **Optional** in the User Input section.
  - If `$ADDITIONAL_CONTEXT` supports structured options, document them.
- **Reference source-of-truth docs** - do NOT duplicate policy boilerplate:
-  - For workflow and gate enforcement, reference `../bubbles/workflows.yaml`.
-  - For cross-project portability and command indirection, reference `../agents/bubbles_shared/project-config-contract.md`.
- **Use tiered context loading** via repo governance docs:
  - Tier 1: Governance (constitution.md, agents.md, copilot-instructions.md)
  - Tier 2: Feature context (spec.md, scopes.md, design.md)
  - Tier 3: Reference docs (on-demand only)
- Avoid drift:
  - If you add/remove a Bubbles command, update the relevant prompt shim under `../prompts/` and keep any command inventories in sync.

## Bubbles Governance (MANDATORY for bubbles.* agents)

All `bubbles.*` agents MUST follow these governance rules and reference canonical docs rather than duplicating policy blocks.

### VS Code Session Repository Context (NON-NEGOTIABLE)

Before any repository-sensitive Bubbles command, resolve host context with the
installed `repository-binding-host-context.sh` (under `bubbles/scripts/` in the
source repo or `.github/bubbles/scripts/` downstream). In VS Code, pass the
host-provided per-chat value exactly as:

```text
--session-log "{{VSCODE_TARGET_SESSION_LOG}}"
```

Also pass every host-declared workspace folder as an explicit
`--workspace-root`. Consume the adapter's `sessionId`, `sessionControlFile`,
`expectedControlRevision`, and canonical `workspaceRoots` in
`repository-binding.sh preflight`; pass `expectedControlRevision` verbatim as
`--expected-control-revision`. If preflight reports a stale revision, rerun the
host adapter and retry with its new observation rather than guessing a value. Never derive
session identity or repository authority from CWD, prompt source, active editor,
workspace order, process ID, generic host repository metadata, or a repo-local
session file. If the host does not resolve the session-log token, refuse before
repository-local reads or writes; there is no fallback.

Key governance references (authoritative sources — do NOT duplicate inline):
- [agent-common.md](../agents/bubbles_shared/agent-common.md) — Anti-fabrication, evidence standards, sequential completion, specialist chain, and quality work standards
- [critical-requirements.md](../agents/bubbles_shared/critical-requirements.md) — Top-priority, non-negotiable policy set (no fabrication, no stubs/TODOs/fallbacks/defaults, full implementation and validation requirements)
- [scope-workflow.md](../agents/bubbles_shared/scope-workflow.md) — DoD templates, phase exit gates, artifact templates, and status ceiling enforcement
- [project-config-contract.md](../agents/bubbles_shared/project-config-contract.md) — What projects must provide, indirection rules, portability inventory, and skills/instructions governance
- [workflows.yaml](../bubbles/workflows.yaml) — Gates, modes, phases, retry policy, and priority scoring
- [feature-templates.md](../agents/bubbles_shared/feature-templates.md) — Version 3 `state.json`, `scenario-manifest.json`, and control-plane artifact templates
- [CONTROL_PLANE_DESIGN.md](../docs/guides/CONTROL_PLANE_DESIGN.md) — Registry-driven delegation, policy defaults, certification ownership, lockdown, and scenario contracts

## Control Plane Requirements (MANDATORY for updated bubbles.* agents)

When creating or updating a `bubbles.*` agent:
- Reference the validate-owned control plane instead of legacy self-promotion language.
- Treat `state.json.execution.*` as execution-claim space and `state.json.certification.*` as validate-owned authoritative completion state.
- If the agent touches user-visible or externally observable behavior planning, reference `scenario-manifest.json` and stable `SCN-*` IDs where appropriate.
- If the agent needs repo-default execution behavior, reference the policy registry in `.specify/memory/bubbles.config.json` and `policySnapshot` provenance instead of inventing local defaults.
- Keep prompt shims and agent definitions in sync so every registered agent has a current prompt surface.

## ⚠️ Universal Anti-Fabrication Contract (MANDATORY for ALL agents)

All agents MUST enforce strict truthfulness and test-substance requirements per [agent-common.md](../agents/bubbles_shared/agent-common.md) and [critical-requirements.md](../agents/bubbles_shared/critical-requirements.md).

**Key rules (see canonical docs for full details):**
- Every pass/fail claim maps to executed command with real output
- Status remains `in_progress`/`blocked` on missing/contradictory evidence
- Apply Fabrication Detection Heuristics (G021), Sequential Spec Completion (G019), Specialist Completion Chain (G022), and Mandatory Completion Checkpoint before reporting complete

### Authoring Requirement

Every new or updated `bubbles.*` agent MUST include a Policy/Compliance section that references:
- `../agents/bubbles_shared/agent-common.md` for anti-fabrication and test-substance rules
- `../bubbles/workflows.yaml` for mode/gate enforcement

### Agent Identity Section (REQUIRED)

Every `bubbles.*` agent MUST include an Agent Identity section immediately after the frontmatter. This establishes the persona and behavioral boundaries that govern the agent's actions.

**Template:**

```markdown
## Agent Identity

**Name:** bubbles.<agent-name>
**Role:** [One-line description of primary responsibility]
**Expertise:** [Key competencies and knowledge domains]

**Behavioral Rules:**
- [Rule 1: How the agent approaches work]
- [Rule 2: Quality/verification standards]
- [Rule 3: Collaboration/handoff behavior]

**Non-goals:**
- [What this agent explicitly does NOT do]
- [Boundaries to prevent scope creep]
```

**Why Required:**
- Establishes clear persona for consistent behavior
- Defines boundaries to prevent scope creep
- Enables proper handoff decisions
- Provides context for LLM role adoption

### Policy & Session Compliance

Include this section in every bubbles agent (do NOT duplicate full rules):

```markdown
## Policy & Session Compliance

Follow policy compliance, session tracking, and context loading per the repo governance files and [project-config-contract.md](../agents/bubbles_shared/project-config-contract.md).

Key requirements:
- Load Tier 1 governance docs first (constitution.md, agents.md, copilot-instructions.md)
- Maintain session state in `bubbles.session.json` with `agent: <agent-name>` when the repo uses a session file
- Respect loop limits and status ceilings from `../bubbles/workflows.yaml`
```

### Action First

Agents MUST take action, not just analyze:
- Load Tier 1 docs → Take ONE action → Load more ONLY if blocked
- Maximum 3 documents before first action
- No analysis loops
For scope completion, test execution, anti-fabrication, and loop limits, reference:
- [agent-common.md](../agents/bubbles_shared/agent-common.md)
- [scope-workflow.md](../agents/bubbles_shared/scope-workflow.md)
- [workflows.yaml](../bubbles/workflows.yaml)

For artifact templates, use:
- [feature-templates.md](../agents/bubbles_shared/feature-templates.md)
- [bug-templates.md](../agents/bubbles_shared/bug-templates.md)
- Max 3 consecutive failures → pause and report

### DoD Evidence Format (MANDATORY)

For all `bubbles.*` agents, DoD completion evidence MUST be embedded directly under each DoD checkbox item in `scopes.md` (or bug `scopes.md`) as raw execution output.

Rules:
- Do NOT use `→ Evidence: [report.md#...]` links for DoD completion.
- Do NOT use summaries or paraphrases as DoD evidence.
- Use verbatim command output with exit code context (≥10 lines for test/run evidence).

### ⚠️ ABSOLUTE: Operation Timeout Policy (NEVER WAIT FOREVER)

**ALL operations executed by agents MUST have explicit time limits. Agents MUST NEVER wait indefinitely.**

See [agent-common.md](../agents/bubbles_shared/agent-common.md) for the full timeout policy, enforcement rules, required patterns, and prohibited patterns.

Key rules: wrap commands with `timeout <duration>`, no infinite loops, health check polling with max attempts (30 × 2s), background processes with max 10 checks.

**On Timeout:**
1. Log the timeout event with context
2. Kill any hung processes
3. Report failure with timeout reason
4. Do NOT retry automatically without explicit user approval

---

## ⚠️ ABSOLUTE: Auto-Approvable Command Patterns Only

Agents MUST avoid command patterns that commonly trigger VS Code Copilot approval prompts.

### PROHIBITED

- `bash -c '...'` or `sh -c '...'` wrappers for normal repo operations
- Chained wrapper commands like `source ... && cd ... && <ecosystem-native test command> ...`
- Temp-file redirection pipelines for output capture (e.g., `> /tmp/x.txt; cat /tmp/x.txt`)

### REQUIRED

1. Use repository-standard entrypoints (resolve `CLI_ENTRYPOINT` from `.specify/memory/agents.md`).