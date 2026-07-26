# AI Environment Configuration

> Team-consistent AI tool configuration for downstream projects using Bubbles.

---

## AGENTS.md vs copilot-instructions.md

Bubbles scaffolds **both** files during bootstrap. They serve complementary purposes:

| File | Location | Loaded By | Best For |
|------|----------|-----------|----------|
| `AGENTS.md` | Repo root | VS Code Copilot, Claude Code, Cursor, Gemini | Short, high-impact guardrails visible to ALL AI tools |
| `.github/copilot-instructions.md` | `.github/` | GitHub Copilot (Chat + Agent) | Detailed project policies, commands, testing requirements |

**Guidance:**
- Keep `AGENTS.md` short (under 100 lines) — architecture summary, key prohibitions, and pointers to detailed docs.
- Put detailed policies, command tables, and testing requirements in `copilot-instructions.md`.
- Both files are committed to the repo and loaded automatically.

---

## MCP Server Configuration

[Model Context Protocol (MCP)](https://modelcontextprotocol.io/) servers extend AI agent capabilities with external tool access — cross-repo search, issue trackers, databases, build systems, etc.

### When to Use MCP

- Your project spans multiple repositories and agents need cross-repo context.
- You use Azure DevOps, GitHub Issues, or other work tracking that agents should reference.
- You have internal tools (search, deployment, monitoring) that agents could query.

### Team Configuration

Create `.vscode/mcp.json` in your repo to share MCP server connections across the team:

```jsonc
{
  "servers": {
    // Example: Cross-repo code search
    "code-search": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-bluebird"]
    },
    // Example: GitHub integration
    "github": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": {
        "GITHUB_PERSONAL_ACCESS_TOKEN": "${input:github_token}"
      }
    }
  }
}
```

**Rules:**
- Commit `.vscode/mcp.json` to the repo so all team members get the same AI tool access.
- Never put secrets directly in the file — use `${input:...}` prompts or environment variable references.
- Document each server's purpose with comments.

### Recommended Extensions

Create `.vscode/extensions.json` to recommend AI-related extensions for the team:

```jsonc
{
  "recommendations": [
    "github.copilot",
    "github.copilot-chat"
  ]
}
```

Commit this file so new team members get consistent tooling suggestions.

---

## Bubbles Integration

Bubbles manages exactly ONE entry in `.vscode/mcp.json`: its own MCP server, registered under a **unique per-repo id** (`bubbles-<repo-slug>`) on every install/upgrade. Every other server in the file, and `.vscode/extensions.json`, stay project-owned — Bubbles never reads or rewrites them. The unique id is what lets the server start cleanly in multi-root workspaces, where a shared generic `bubbles` id across folders makes VS Code's MCP gateway refuse to start any of them. See [MCP.md](../MCP.md) for details.

Bubbles provides the agent definitions, skills, and governance; MCP provides external tool access that complements them.

**Example workflow:**
1. Agent loads `copilot-instructions.md` (project rules) + `AGENTS.md` (guardrails)
2. Agent loads relevant skill from `.github/skills/` (domain knowledge)
3. Agent uses MCP server to query external system (cross-repo search, issue tracker)
4. Agent follows Bubbles governance (anti-fabrication, evidence standards) for all outputs

---

## Multi-Root Workspaces: Agent Source, Work Repository, And Task Scope

When several Bubbles-installed repositories are open in one multi-root workspace, the same agent name (`bubbles.goal`, `bubbles.analyst`, ...) exists under **every** workspace root. Bubbles keeps three independent boundaries explicit:

1. **Agent source binding** proves that the loaded agent belongs to the repository being edited.
2. **Work-repository binding** selects the one canonical Git worktree a repository-sensitive command may inspect or mutate in the current interactive session.
3. **Task work boundary** limits which repositories, specs, and paths that already-bound command may change.

None of these boundaries can be inferred from chat/process/tool CWD, prompt location, the active editor, recent files, host `repository` metadata, workspace order, or incidental file access.

### What Bubbles controls (mechanical)

| Surface | What it does |
|---------|--------------|
| Per-repo MCP id `bubbles-<repo-slug>` | Disambiguates the framework **server** so the MCP gateway starts cleanly per root (see [MCP.md](../MCP.md)). |
| `.github/bubbles/.install-source.json` `targetRepoSlug` marker | A repo-relative identity token stamped by `install.sh` on every install/upgrade — never a per-machine absolute path. |
| `bubbles/scripts/repo-binding-preflight.sh` | Fails loud (exit 1) before mutable work when the active agent's source-repo slug does not match the repository being edited; passes for `--canonical-source` framework work; advisory (exit 0) when no marker is present yet. |
| `bubbles/scripts/repository-binding.sh` | Resolves and atomically commits the current session's work repository before repository-local state, relative targets, `specs/` discovery, repository commands, or specialist dispatch. It also validates child packets, scopes discovery, and mirrors the committed decision after selection. |
| `bubbles/scripts/work-boundary-resolve.sh` | Classifies a candidate change (repo / spec / path) against a feature's declared `workBoundary` (IMP-100 R6): `in-boundary`, `route-same-repo` (unrelated same-repo → file/route, never inline-fix), `route-cross-repo` (`crossRepoPolicy: authorized`), or `refuse-cross-repo` (a different repo under the default `forbidden` policy). Backward-compatible (no boundary → `in-boundary`); fail-closed (exit 2) on a malformed boundary. Guards the per-task allowed scope; composes with the repo-binding preflight above. |
| Provenance-bearing work packets | Resolution, work, dispatch, result, continuation, recap, status, handoff, compaction, and goal-node packets carry the exact repository decision. Consumers validate it against the current external session control record before any repository-local action. |
| Mechanical binding over a cosmetic label | Bubbles does **not** inject a per-repo qualifier into agent `.agent.md` files — those install byte-identical to canonical source (a managed-file integrity/tamper-detection invariant; see the `install-provenance` selftest's "installed bytes match canonical source" check). The mechanical boundaries above are stronger than a cosmetic picker suffix. |

### Work-Repository Authority

`repository-binding.sh preflight` uses this closed authority order:

1. one valid explicit `repositoryRoot` or exact concrete target;
2. one valid durable work boundary established by successful targeted work earlier in the same host session;
3. the sole eligible canonical repository in a true single-repository inventory.

There is no multi-root first-folder fallback. A mode-only request is classified as `TARGETLESS_MODE`, including `mode:` plus `repositoryRoot` without a concrete spec/bug/ops target. Repository preflight commits first; only then are mode-specific target rules evaluated. For a mode that permits discovery, the executable pool is exactly `<resolved-repository-root>/specs`:

```text
/bubbles.workflow  mode: stochastic-quality-sweep repositoryRoot: <canonical-repository-root>
DISCOVERY SCOPE mode=stochastic-quality-sweep root=<canonical-repository-root>/specs
```

A successful targeted command establishes, confirms, or switches the durable boundary. Later targetless continuation, status, handoff, recap, and iterate operations validate and continue that same decision. A distinct interactive session does not inherit an old boundary merely because `.specify/memory/bubbles.session.json` remains on disk; the repository-local mirror is post-selection state, never authority.

Ambiguous or stale authority refuses before local reads with a stable reason, `requiredInput.field: repositoryRoot`, `affinity: unchanged`, and `repoLocalSideEffects: zero`. The remediation is an explicit canonical root, not a CWD/editor/workspace-order change.

Local same-session packets carry the canonical root with `pathVisibility: local` and `actionable: true`. Public or committed projections use `repositoryRoot: <redacted-local-root>`, `pathVisibility: redacted`, and `actionable: false`; they are safe to share but cannot resume or dispatch work.

Cross-repository goal/sprint nodes use explicit per-node canonical roots and `scopeKind: goal-node`. A node decision is a scoped override and never changes top-level session affinity. Selecting a downstream repository also does not authorize edits to installed framework files: framework changes remain upstream-first in the canonical Bubbles source repository.

Run both preflights before cross-repo edits (or let an orchestrator run them): work-repository binding selects where work occurs, and agent-source binding confirms that the loaded agent is allowed to edit there.

### What Bubbles does NOT control (upstream editor limitation)

VS Code **Chronicle** (session attribution) attributes a multi-root session's work to the **first** workspace root, regardless of which repository the agent actually edited. This is an editor attribution behavior; Bubbles cannot fix it unilaterally from inside a downstream repo. Bubbles records the true binding where it CAN (the `targetRepoSlug` marker + the preflight + the envelope provenance fields), but the Chronicle roll-up remains an upstream limitation — treat Chronicle's per-root attribution as advisory in multi-root workspaces, and trust the Bubbles binding surfaces above for the authoritative agent↔repo relationship.

---

## See Also

- [INSTALLATION.md](INSTALLATION.md) — Full Bubbles installation guide
- [AGENT_MANUAL.md](AGENT_MANUAL.md) — Agent usage and workflow guide
- [project-config-contract.md](../../agents/bubbles_shared/project-config-contract.md) — What projects must provide
