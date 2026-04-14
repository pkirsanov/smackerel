# Operating Baseline

Use this file for shared operating behavior instead of duplicating the same session/loading/loop prose in prompts.

## Project-Agnostic Indirection

Agents MUST resolve project-specific commands, ports, paths, and policy details through `.specify/memory/agents.md`, `.specify/memory/constitution.md`, and `.github/copilot-instructions.md`. Do not hardcode project-specific values into portable prompts.

## Framework File Immutability — Upstream-First (NON-NEGOTIABLE)

**Agents MUST NEVER create, modify, or delete Bubbles framework-managed files inside downstream project repos.** These files are owned exclusively by the canonical Bubbles repository and propagated to downstream projects only through `install.sh` upgrades.

**Upstream-First Flow (ABSOLUTE):** ALL Bubbles framework changes — governance docs, agent definitions, shared modules, scripts, workflows, instructions, skills, prompts — MUST be authored in the **canonical Bubbles repository**. Downstream projects receive these updates via the upgrade command (`bash .github/bubbles/scripts/cli.sh upgrade`). Agents MUST NOT edit framework-managed files in downstream repos, and MUST NOT manually copy or sync framework files between repos.

**Multi-Root Workspace Rule:** When working in a multi-root workspace that contains both the canonical Bubbles repo and one or more downstream projects, all framework file edits go to the Bubbles repo. The `.github/` copies in downstream repos are read-only install artifacts — not authoring targets.

Downstream repos may request framework changes via `.github/bubbles-project/proposals/` or `bubbles framework-proposal <slug>`, but they MUST NOT directly edit framework-managed files.

### Framework-Managed Paths (READ-ONLY for agents)

| Path | Owner | Update Mechanism |
|------|-------|------------------|
| `.github/agents/bubbles.*.agent.md` | Bubbles framework | `install.sh` |
| `.github/agents/bubbles_shared/*.md` | Bubbles framework | `install.sh` |
| `.github/bubbles/scripts/*.sh` | Bubbles framework | `install.sh` |
| `.github/bubbles/workflows.yaml` | Bubbles framework | `install.sh` |
| `.github/bubbles/hooks.json` | Bubbles framework | `install.sh` |
| `.github/bubbles/agnosticity-allowlist.txt` | Bubbles framework | `install.sh` |
| `.github/bubbles/*.yaml` (except `bubbles-project.yaml`) | Bubbles framework | `install.sh` |
| `.github/prompts/bubbles.*.prompt.md` | Bubbles framework | `install.sh` |
| `.github/instructions/bubbles-*.instructions.md` | Bubbles framework | `install.sh` |
| `.github/skills/bubbles-*/SKILL.md` | Bubbles framework | `install.sh` |

### Project-Owned Paths (agents MAY modify)

| Path | Owner | Purpose |
|------|-------|---------|
| `.github/bubbles-project.yaml` | Project | Custom quality gates and scan patterns |
| `.github/bubbles-project/proposals/**` | Project | Proposed upstream Bubbles changes requested by this repo |
| `.github/copilot-instructions.md` | Project | Project-specific policies |
| `.specify/memory/agents.md` | Project | CLI entrypoint, commands, naming |
| `.specify/memory/constitution.md` | Project | Project governance principles |
| `specs/**` | Project | Classified work artifacts (feature, bug, ops) |

### What To Do Instead

| Need | Action |
|------|--------|
| Fix a framework script bug | Run `bubbles framework-proposal <slug>` or add a proposal under `.github/bubbles-project/proposals/`, then implement it upstream in the Bubbles repository |
| Add a project-specific quality check | Add to `scripts/` or `.github/bubbles-project.yaml` custom gates |
| Add project-specific scan patterns | Edit `.github/bubbles-project.yaml` `scans:` section |
| Need an agnosticity-lint exception or framework allowlist change | Propose the framework change upstream instead of editing `.github/bubbles/agnosticity-allowlist.txt` locally |

### Violation Detection

The `agnosticity-lint.sh --staged` pre-commit check detects project-specific content in framework files. The downstream `framework-write-guard` verifies that framework-managed files still match the last installed upstream checksum snapshot. Additionally, `install.sh` upgrades will overwrite local modifications, causing silent regression if agents modify framework files locally.

## Loop Guard

1. Start with the smallest role bootstrap that fits the job.
2. Take one real action after the minimum initial context set is loaded.
3. No redundant rereads without a new reason.
4. One feature-resolution attempt before failing fast on an ambiguous or missing target.
5. Read only the files needed for the current phase, gate, or claim.

## Context Loading Profiles

- `planner`: `plan-bootstrap.md`
- `implementer`: `implement-bootstrap.md`
- `tester`: `test-bootstrap.md`
- `analyst`: `analysis-bootstrap.md`
- `designer`: `design-bootstrap.md`
- `docs`: `docs-bootstrap.md`
- `clarifier`: `clarify-bootstrap.md`
- `ux`: `ux-bootstrap.md`
- `validator`: `audit-bootstrap.md` plus project command sources as needed
- `auditor`: `audit-bootstrap.md`
- `orchestrator`: `bubbles/workflows.yaml`, `state.json`, the scope entrypoint, and only the dispatch metadata required for the active step
- `simplifier`: `implement-bootstrap.md`
- `chaos`: `test-bootstrap.md`

## Autonomous Operation

- Non-interactive by default unless the prompt explicitly opts into bounded questioning.
- Fix the smallest blocked unit first, then re-run the narrowest relevant verification.
- Route foreign-artifact changes to the owning specialist instead of editing them inline.
- **Honesty over completion:** When evidence is ambiguous, prefer leaving a DoD item `[ ]` with an Uncertainty Declaration over marking `[x]` with uncertain evidence. A wrong answer is 3x worse than an honest gap. See `critical-requirements.md` → Honesty Incentive.
- **Evidence provenance:** Every evidence block must include a `**Claim Source:**` tag (`executed`, `interpreted`, `not-run`). See `evidence-rules.md` → Evidence Provenance Taxonomy.

## Auto-Approval And Timeouts

- Avoid shell wrapper patterns that trigger approval prompts unless explicitly required.
- Every long-running operation must have an explicit timeout or bounded polling rule.

## Classified Work Resolution

- Work only inside classified `specs/...` feature, bug, or ops targets.
- If the target is not found after one resolution attempt, fail fast and report the valid alternatives.