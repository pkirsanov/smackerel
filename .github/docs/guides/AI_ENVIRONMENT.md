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

Bubbles does NOT manage `.vscode/mcp.json` or `.vscode/extensions.json` — these are project-owned files. Bubbles provides the agent definitions, skills, and governance; MCP provides external tool access that complements them.

**Example workflow:**
1. Agent loads `copilot-instructions.md` (project rules) + `AGENTS.md` (guardrails)
2. Agent loads relevant skill from `.github/skills/` (domain knowledge)
3. Agent uses MCP server to query external system (cross-repo search, issue tracker)
4. Agent follows Bubbles governance (anti-fabrication, evidence standards) for all outputs

---

## See Also

- [INSTALLATION.md](INSTALLATION.md) — Full Bubbles installation guide
- [AGENT_MANUAL.md](AGENT_MANUAL.md) — Agent usage and workflow guide
- [project-config-contract.md](../../agents/bubbles_shared/project-config-contract.md) — What projects must provide
