# Using Bubbles with Codex

Bubbles keeps its Copilot integration under `.github/`, while Codex discovers
repository instructions from `AGENTS.md`, reusable skills from `.agents/skills`,
custom agents from `.codex/agents`, and local MCP servers from
`.codex/config.toml`.

## Source checkout

The Bubbles source repository already ships these Codex-native files:

- `AGENTS.md`
- `.agents/skills` → `skills` (a symlink)
- `.codex/agents/bubbles_*.toml`
- `.codex/config.toml`

Open the repository with Codex, then use a matching skill or ask explicitly for
one of the `bubbles_*` specialist agents. The source MCP server starts through
the checked-in `.codex/config.toml` configuration.

## Downstream installation

After installing Bubbles into a product repository, add these project-owned
files. They are deliberately outside `.github/`, so a Bubbles upgrade will not
overwrite them.

1. Create a root `AGENTS.md` with the product's commands and conventions.
2. Link the installed portable skills into Codex's discovery location:

   ```sh
   mkdir -p .agents
   ln -s ../.github/skills .agents/skills
   ```

3. Copy the desired agent files from the Bubbles source's `.codex/agents/` into
   the product's `.codex/agents/` directory.
4. Merge this MCP entry into `.codex/config.toml`:

   ```toml
   [mcp_servers.bubbles]
   command = "python3"
   args = [".github/bubbles/mcp/server.py"]
   startup_timeout_sec = 20
   tool_timeout_sec = 300
   default_tools_approval_mode = "writes"
   ```

Restart Codex after adding skills, agents, or MCP configuration. Do not copy the
Copilot `*.agent.md` frontmatter into Codex; it is not a Codex agent format.
