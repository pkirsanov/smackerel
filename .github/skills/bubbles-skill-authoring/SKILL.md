---
name: bubbles-skill-authoring
description: Guidance for creating or updating repo-local skills under .github/skills. Use when adding procedural workflows, checklists, or reusable resources that must follow repository governance.
---

# Bubbles Skill Authoring

## Goal
Create high-signal skills that improve agent performance without bloating context or violating repository policies.

## Portability
This is a portable governance skill. Keep it free of project-specific commands, hostnames, ports, and repo-only workflows.

## Non-negotiables
- Do not hardcode environment-specific hosts, URLs, or ports.
- Do not invent defaults or fallbacks that hide missing configuration.
- Do not prescribe ad-hoc build or run commands. Resolve command execution through `.specify/memory/agents.md` and repository policy docs.
- Avoid embedding opaque scripting blobs inside shell scripts unless the target repository explicitly allows it.
- When a skill touches spec/workflow/test guidance, reference the control-plane artifacts (`policySnapshot`, `scenario-manifest.json`, validate-owned `certification.*`) instead of teaching legacy completion/state semantics.

## Skill Structure
A skill is a folder under `.github/skills/<skill-name>/`.

Required:
- `SKILL.md` with YAML frontmatter:
  - `name`: lowercase, hyphen-separated
  - `description`: concrete activation triggers

Optional bundled resources:
- `references/`: small, task-relevant docs used only when needed
- `scripts/`: deterministic helpers
- `assets/`: templates or reusable artifacts

## Writing Guidelines
- Keep `SKILL.md` short and action-oriented.
- Use imperative language.
- Prefer checklists and decision trees over long prose.
- Rewrite adapted upstream guidance in repository-neutral terms.

## Progressive Disclosure
- Put workflow and navigation in `SKILL.md`.
- Put schemas, examples, and long lists into `references/`.
- Link references explicitly and say when to open them.

## Tooling and File Operations
- Prefer editor-native file tools for creating and editing files.
- Do not tell users to run shell commands to create files that the agent can create directly.

## Quality Bar
A skill is done when:
- It does not conflict with `.github/copilot-instructions.md`.
- It introduces no forbidden defaults, hosts, or ports.
- It routes execution through repository-standard workflows.
- It improves repeatability.

## References
- `.github/instructions/bubbles-skills.instructions.md`
- `.github/agents/bubbles_shared/project-config-contract.md`
- `.github/agents/bubbles_shared/agent-common.md`
- `.github/agents/bubbles_shared/feature-templates.md`
- `docs/guides/CONTROL_PLANE_DESIGN.md`