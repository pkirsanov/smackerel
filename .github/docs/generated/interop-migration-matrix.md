# Interop Migration Matrix

Capability context: 4 shipped, 1 partial, 2 proposed.

This page is generated from `bubbles/capability-ledger.yaml` and `bubbles/interop-registry.yaml` so evaluators can compare supported apply, review-only intake, and proposal-only boundaries without relying on hand-maintained competitor prose.

| Source | Parser | Review-Only Intake | Supported Apply Targets | Proposal-Only / Unsupported |
| --- | --- | --- | --- | --- |
| Claude Code | markdown | CLAUDE.md, .claude/commands/, .claude/agents/ | .github/instructions/imported-<source>.instructions.md, .github/instructions/imported-<source>.instructions.md, .specify/memory/agents.md, scripts/imported-<source>-tooling.md, .github/skills/imported-<source>-migration/SKILL.md | .github/agents/bubbles*, .github/prompts/bubbles*, .github/skills/bubbles-*, .github/bubbles/** |
| Roo Code | hybrid | .roo/rules/, .roo/modes/, .roomodes | .github/instructions/imported-<source>.instructions.md, .github/instructions/imported-<source>.instructions.md, scripts/imported-<source>-tooling.md, .github/agents/bubbles*, .github/prompts/bubbles*, .github/skills/bubbles-*, .github/bubbles/** | — |
| Cursor | markdown | .cursor/rules/, .cursorrules | .github/instructions/imported-<source>.instructions.md, .github/instructions/imported-<source>.instructions.md, .github/skills/imported-<source>-migration/SKILL.md, .github/agents/bubbles*, .github/prompts/bubbles*, .github/skills/bubbles-*, .github/bubbles/** | — |
| Cline | markdown | .clinerules, .cline/rules/, .cline/modes/ | .github/instructions/imported-<source>.instructions.md, .github/instructions/imported-<source>.instructions.md, .github/skills/imported-<source>-migration/SKILL.md, .github/agents/bubbles*, .github/prompts/bubbles*, .github/skills/bubbles-*, .github/bubbles/** | — |
