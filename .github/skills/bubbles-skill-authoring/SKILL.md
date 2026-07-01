---
name: bubbles-skill-authoring
description: Guidance for creating or updating repo-local skills under .github/skills. Use when adding procedural workflows, checklists, or reusable resources that must follow repository governance.
---

# Bubbles Skill Authoring

## Goal
Create high-signal skills that improve agent performance without bloating context or violating repository policies.

## Portability
This is a portable governance skill. Keep it free of project-specific commands, hostnames, ports, and repo-only workflows.

## Scope: project vs personal
Project-scoped procedures live in `.github/skills/` and travel with the repository. Personal or cross-repo procedural preferences do NOT belong in a project skill — they live in the observation-driven developer profile (`.specify/memory/developer-profile.md`) and the agent's user-memory layer. Do not stand up a parallel personal-skills surface; route personal preferences to those existing layers instead.

## Placement & Naming
Skill placement is classified by CONTENT, not by a name prefix. The authority is [`project-config-contract.md`](../../agents/bubbles_shared/project-config-contract.md) § **Skills Classification**:

- **Portable** — hardcodes no project-specific path/tool/CLI (uses `agents.md` indirection); usable in any repo. The framework's own portable skills use the `bubbles-` name prefix by convention, but portability is about content, not the prefix (`bug-fix-testing/` is unprefixed yet classified portable).
- **Project-specific** — references a project CLI, project services, or a domain-specific pattern. Named with a domain term (`chaos-execution`, `protobuf-only`) or a repo-descriptive `<repo>-` prefix; lives only in that repo and MUST NOT be assumed to exist elsewhere.

Naming guidance (a convention, NOT a mechanical rename mandate):
- Reserve the `bubbles-` prefix for framework-portable skills — do not put it on a skill that hardcodes one repo's specifics.
- Both unprefixed domain names and `<repo>-*` are established, acceptable forms for a project-specific skill; there is no requirement to prefix every project skill with the repo name.
- Some framework agents PIN a project-skill by a fixed path (e.g. `bubbles.chaos.agent.md` loads `.github/skills/chaos-execution/SKILL.md`). NEVER rename a skill a framework agent pins — its name is a contract.

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

## Recommended Body Sections
Beyond the workflow itself, a `SKILL.md` SHOULD include these two sections when applicable. Both are RECOMMENDED, not mandatory — existing skills without them remain valid:
- **When NOT to use** — explicit negative triggers that route to the correct sibling skill. Stating where the skill does *not* apply sharpens semantic auto-load and reduces wrong-skill activation when several skills share vocabulary.
- **Works well with** — composition pointers to the sibling skills this one commonly chains with, so a reader knows the natural next or previous step.

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

## When to promote a procedure to a skill
Decision rule: **Do it once → a prompt is fine. Recurring + non-obvious + verified → promote to a skill.**

A one-off instruction belongs in a prompt or a chat message, not a skill. Promote a procedure only when it is *recurring* (you have reached for it more than once), *non-obvious* (it encodes judgment a fresh agent would not guess), and *verified* (you have watched it actually work). This is the same trigger the Skill Evolution Loop applies when it surfaces a repeated-lesson proposal (`skill-evolution.sh` → `.specify/memory/skill-proposals.md`), and the inverse of the `INVENTORY.md` pruning policy that deprecates stale skills. Treat promotion and pruning as the two ends of one lifecycle, governed by the same bar.

## Quality Bar
Before a candidate becomes a skill, it must clear the creation bar — **Reusable · Non-trivial · Specific · Verified**:
- **Reusable** — it applies beyond the single situation that prompted it.
- **Non-trivial** — it encodes judgment or sequencing a fresh agent would not guess.
- **Specific** — it targets a concrete task with concrete triggers, not a vague theme.
- **Verified** — only codify what actually worked; do not transcribe untested theory or duplicate official docs.

A skill is done when:
- It does not conflict with `.github/copilot-instructions.md`.
- It introduces no forbidden defaults, hosts, or ports.
- It routes execution through repository-standard workflows.
- It improves repeatability.

## Optional bin/ and install.sh Conventions

Skills MAY ship executable helpers in a `bin/` subdirectory:

```
.github/skills/<skill-name>/
├── SKILL.md
└── bin/
    ├── <helper-name>          # executable, no extension
    └── <other-helper>.sh
```

**Rules for `bin/` helpers:**
- MUST be executable (`chmod +x`) and idempotent
- MUST run with no installation (no `npm install`, no `pip install` at runtime)
- MUST use only tools available in a baseline POSIX shell + standard CLI (bash, grep, awk, sed, jq if declared)
- SHOULD honor a `--help` flag and a `--version` flag
- SHOULD NOT mutate the workspace silently; print what they will change

Skills that need installation (compiled binaries, vendored dependencies) MAY ship an `install.sh`:

```
.github/skills/<skill-name>/
├── SKILL.md
├── install.sh                 # idempotent installer
└── bin/
    └── <installed-tool>
```

**Rules for `install.sh`:**
- MUST be idempotent (safe to run twice)
- MUST detect existing install and skip rather than reinstall
- MUST print what it installs and where
- MUST NOT require root
- SHOULD verify installed tool with a checksum or signature

## References
- `.github/instructions/bubbles-skills.instructions.md`
- `.github/agents/bubbles_shared/project-config-contract.md`
- `.github/agents/bubbles_shared/agent-common.md`
- `.github/agents/bubbles_shared/feature-templates.md`
- `docs/guides/CONTROL_PLANE_DESIGN.md`