# Skill File Guidelines

> Adapted from [github/awesome-copilot](https://github.com/github/awesome-copilot) (MIT License).
> Modified to be repository-agnostic and to defer to this repository's policies.
>
> **Portability:** This file is **project-agnostic**. Copy unchanged across projects.
> See [project-config-contract.md](../agents/bubbles_shared/project-config-contract.md) for the cross-project portability rules.
> See [agent-common.md](../agents/bubbles_shared/agent-common.md) for universal governance (anti-fabrication, evidence standards).

## What Are Skills?

Skills provide domain-specific knowledge that agents can load on demand. They're ideal for:

- Procedural workflows and checklists
- Domain expertise (trading patterns, risk models, etc.)
- Reusable templates and patterns
- Reference documentation

## Skill Discovery

- Discover available skills from `.github/skills/*/SKILL.md`.
- Treat skill availability as repository-specific.
- Do not assume cross-repo skill names or file paths unless they exist in the current repository.
- Keep this instruction file project-agnostic; put repo-specific skill inventories in repo docs if needed.

## Skill Directory Structure

Skills live in `.github/skills/<skill-name>/`:

```
.github/skills/
└── my-skill/
    ├── SKILL.md           # Required: Main skill file
    ├── references/        # Optional: Supporting docs
    │   └── patterns.md
  ├── scripts/           # Optional: Shell scripts (use the repo-standard runner/workflow)
    └── assets/            # Optional: Templates, configs
```

## SKILL.md Format

### Required Frontmatter

```yaml
---
name: skill-name
description: One-line description of what this skill provides
---
```

### Optional Frontmatter

```yaml
---
name: skill-name
description: Required description
globs:
  - "**/*.rs"
  - "services/**"
applyTo: agent  # or "prompt"
---
```

## Writing Guidelines

### Progressive Disclosure

1. **SKILL.md**: High-level workflow and key rules (scannable in <30 seconds)
2. **references/**: Detailed documentation for specific topics
3. **assets/**: Templates, examples, boilerplate

### Content Style

- Use **imperative language**: "Run validation" not "You should run validation"
- Be **action-oriented**: Focus on what to do, not background
- Keep **short**: If a section exceeds 50 lines, split into references/
- **Prioritize negative rules**: Explicit prohibitions ("NEVER do X", "FORBIDDEN: Y") yield the biggest quality improvements. Focus on rules the AI cannot infer from code alone — architectural boundaries, cross-layer restrictions, naming conventions, and security guardrails. Studies show negative rules are more effective than positive guidance at preventing AI mistakes.

### Example SKILL.md Structure

```markdown
## Purpose
Brief statement of what this skill enables.

## Quick Start
1. Step one
2. Step two
3. Step three

## Key Rules
- Critical constraint
- Another constraint

## Common Patterns

### Pattern A
Description and usage.

### Pattern B
Description and usage.

## References
- Detailed Topic (references/topic.md)
```

## Repository Policy Hook (MANDATORY)

Skills MUST NOT:

1. **Include localhost/hardcoded ports** - Use configuration references
2. **Embed shell scripts directly** - Put scripts in `scripts/` and invoke them via the repo-standard runner/workflow
3. **Contain stubs or placeholder implementations**
4. **Define default values** - Reference configuration files instead
5. **Allow infinite waits** - All operations MUST have explicit timeouts
6. **Allow unverified claims** - Any skill step that produces a pass/fail result MUST require actual execution with captured output
7. **Hardcode project-specific commands** - Use `[cmd]` placeholders and resolve from `.specify/memory/agents.md`
8. **Duplicate governance rules** - Reference `agent-common.md` instead of copying rules inline

Skills MUST:

1. **Reference `.github/copilot-instructions.md`** for policy compliance
2. **Use the repo-standard runner/workflow** for builds/tests (do not hardcode commands)
3. **Enforce operation timeouts** - Never wait forever for any operation
4. **Use configuration from** `.specify/memory/agents.md` (command registry) and project config
5. **Require execution evidence** - Any skill that involves verification (testing, validation, health checks) MUST mandate actual tool/terminal execution and recording of real output
6. **Reference `agent-common.md`** for anti-fabrication gates (G019-G022), evidence standards, and quality work standards
7. **State portability status** - Whether the skill is portable (governance) or project-specific (domain)
8. **Acknowledge the control plane when relevant** - Skills that guide spec/design/test/workflow work must point readers to the version 3 `state.json` template, `scenario-manifest.json`, validate-owned certification, and policy provenance surfaces instead of teaching legacy completion semantics

## Execution Evidence Policy (MANDATORY for Verification Skills)

**Skills that involve testing, validation, deployment verification, or health checks MUST require actual execution evidence.**

Enforce the Execution Evidence Standard and Anti-Fabrication Policy in [agent-common.md](../agents/bubbles_shared/agent-common.md), including Gates G019-G022.

Minimum requirements:
- Commands MUST be executed.
- Raw terminal output (≥10 lines) MUST be captured.
- Exit codes MUST be verified from actual output.
- No summaries or fabricated evidence.
- **Anti-fabrication heuristics (G021) apply** — evidence must pass depth, template, batch, and summary language checks.
- **Sequential completion (G019) inherited** — skills invoked during scope work inherit the sequential completion requirement.

## Governance Enforcement in Skills (MANDATORY)

> **Authoritative source:** [project-config-contract.md](../agents/bubbles_shared/project-config-contract.md) → Skills Governance Requirements.

All skills — portable or project-specific — MUST enforce these universal policies:

| Policy | Gate | What Skills Must Do |
|--------|------|--------------------|
| Anti-Fabrication | G021 | Verification steps require actual execution evidence. No summaries. |
| Evidence Standard | G005 | ≥10 lines raw terminal output for pass/fail claims |
| Operation Timeouts | — | All commands wrapped with explicit timeout protection |
| Sequential Completion | G019 | Skills invoked during scope work inherit sequential completion |
| Quality Work Standards | — | No stubs, placeholders, or fake data in skill outputs |
| Specialist Chain | G022 | Skills that feed into specialist phases inherit completion requirements |

**Portable skills** (governance-only) MUST:
- Contain zero project-specific commands, paths, or tools
- Use `agents.md` indirection for commands
- Reference `agent-common.md` for governance policies

**Project-specific skills** (domain workflows) MAY:
- Reference project-specific commands and paths
- BUT must still enforce all governance policies above

## Operation Timeout Policy (MANDATORY)

**All operations invoked by skills MUST have explicit time limits. Skills MUST NEVER instruct waiting indefinitely.**

Use the timeout rules and patterns from [agent-common.md](../agents/bubbles_shared/agent-common.md).

## Skill Registration

Skills are automatically discovered when placed in `.github/skills/`. No explicit registration required.

To reference a skill in an agent:

```yaml
---
description: Agent that uses a skill
---

Load the bubbles-trading-patterns skill for domain knowledge.

<skill>bubbles-trading-patterns</skill>
```

## Quality Checklist

Before submitting a skill:

- [ ] `SKILL.md` has required frontmatter (`name`, `description`)
- [ ] Content is scannable quickly
- [ ] No hardcoded URLs, ports, or localhost references
- [ ] Any scripts are invoked via the repo-standard workflow
- [ ] No conflicts with `.github/copilot-instructions.md`
- [ ] Progressive disclosure is used appropriately

## See Also

- [workflows.yaml](../bubbles/workflows.yaml)
- [agent-common.md](../agents/bubbles_shared/agent-common.md)
- [scope-workflow.md](../agents/bubbles_shared/scope-workflow.md)
- [project-config-contract.md](../agents/bubbles_shared/project-config-contract.md)
- [feature-templates.md](../agents/bubbles_shared/feature-templates.md)
- [CONTROL_PLANE_DESIGN.md](../docs/guides/CONTROL_PLANE_DESIGN.md)
- [bubbles-skill-authoring](../skills/bubbles-skill-authoring/SKILL.md)
- [bubbles-agents.instructions.md](bubbles-agents.instructions.md)
- Repository `copilot-instructions.md`