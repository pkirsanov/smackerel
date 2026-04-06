<p align="center">
  <img src="icons/bubbles-glasses.svg" width="120" height="120" alt="Bubbles">
</p>

<h1 align="center"><img src="icons/bubbles-glasses.svg" width="32" height="32"> Bubbles</h1>

<p align="center">
  <strong>AI Agent Orchestration System for VS Code Copilot</strong><br>
  <em>"It ain't rocket appliances, but it works."</em>
</p>

<p align="center">
  <!-- GENERATED:FRAMEWORK_STATS_BADGES_START -->
  <img src="https://img.shields.io/badge/agents-34-58a6ff?style=flat-square" alt="34 agents">
  <img src="https://img.shields.io/badge/gates-60-3fb950?style=flat-square" alt="60 gates">
  <img src="https://img.shields.io/badge/workflow_modes-29-bc8cff?style=flat-square" alt="29 modes">
  <!-- GENERATED:FRAMEWORK_STATS_BADGES_END -->
  <img src="https://img.shields.io/badge/fabrication_tolerance-zero-f85149?style=flat-square" alt="zero fabrication">
  <img src="https://img.shields.io/badge/license-MIT-d29922?style=flat-square" alt="MIT">
</p>

<p align="center">
  <a href="https://pkirsanov.github.io/bubbles/docs/its-not-rocket-appliances.html"><strong>Visual Cheatsheet</strong></a> · <a href="docs/CHEATSHEET.md">Markdown Cheatsheet</a> · <a href="docs/guides/AGENT_MANUAL.md">Agent Manual</a> · <a href="docs/CATALOG.md">Recipe Catalog</a> · <a href="docs/recipes/">Recipes</a>
</p>

---

## What Is This?

Bubbles is a **spec-driven AI agent orchestration system** for VS Code Copilot Chat. It turns your `/` slash commands into a full software delivery pipeline — from business analysis to implementation to testing to audit — with zero tolerance for fabricated work, plus a control plane that tracks certification authority, scenario contracts, workflow run-state, typed framework events, runtime lease safety, and framework-level validation.

**One entry point. Just describe what you want:**

```
/bubbles.workflow  improve the booking feature to be competitive
/bubbles.workflow  fix the calendar bug
/bubbles.workflow  continue
/bubbles.workflow  spend 2 hours on whatever needs attention
```

Workflow resolves your intent, picks the right mode, and drives specialists to completion. No need to memorize agents, modes, or parameters.

Think of it as a trailer park supervisor for your codebase. Except this one actually works.

<table>
<!-- GENERATED:FRAMEWORK_STATS_CALLOUTS_START -->
<tr><td width="64"><img src="icons/bubbles-glasses.svg" width="48"></td><td><strong>34 specialized agents</strong> — each with a defined role, from implementation to framework ops</td></tr>
<tr><td width="64"><img src="icons/lahey-badge.svg" width="48"></td><td><strong>60 quality gates</strong> — nothing ships without evidence. Nothing.</td></tr>
<tr><td width="64"><img src="icons/julian-glass.svg" width="48"></td><td><strong>29 workflow modes</strong> — from full delivery to quick bugfixes to chaos sweeps</td></tr>
<!-- GENERATED:FRAMEWORK_STATS_CALLOUTS_END -->
<tr><td width="64"><img src="icons/barb-keys.svg" width="48"></td><td><strong>Optional execution tags</strong> — opt into grilling, inner-loop TDD, backlog export, Socratic discovery, git isolation, atomic commits, scope sizing, and micro-fix loops without weakening baseline planning gates</td></tr>
<tr><td width="64"><img src="icons/lahey-badge.svg" width="48"></td><td><strong>Framework ops surface</strong> — health checks, framework validation, release hygiene, runtime coordination, and optional repo-readiness guidance live behind `bubbles.super` and the CLI</td></tr>
</table>

---

## Install

One command. No dependencies beyond `curl` and `bash`.

**Supported platforms:** VS Code + GitHub Copilot Chat (required). Works on macOS, Linux, and WSL2. No Windows CMD/PowerShell support.

```bash
# Install shared Bubbles framework files
curl -fsSL https://raw.githubusercontent.com/pkirsanov/bubbles/main/install.sh | bash

# Install agents only (skip shared instructions and skills)
curl -fsSL https://raw.githubusercontent.com/pkirsanov/bubbles/main/install.sh | bash -s -- --agents-only

# Install + scaffold project config (recommended for new projects)
curl -fsSL https://raw.githubusercontent.com/pkirsanov/bubbles/main/install.sh | bash -s -- --bootstrap

# Bootstrap with explicit project name and CLI
curl -fsSL https://raw.githubusercontent.com/pkirsanov/bubbles/main/install.sh | bash -s -- --bootstrap --cli ./myproject.sh --name "My Project"
```

Pin to a version:

```bash
curl -fsSL https://raw.githubusercontent.com/pkirsanov/bubbles/v1.0.0/install.sh | bash -s -- --bootstrap
```

Update:

```bash
# Same command. It overwrites the shared files, leaves your project config alone.
curl -fsSL https://raw.githubusercontent.com/pkirsanov/bubbles/main/install.sh | bash
```

### What `--bootstrap` Does

With `--bootstrap`, the installer goes beyond the shared framework files and scaffolds a fully working project setup:

1. **Auto-detects** your project name (from git/directory) and CLI entrypoint (`*.sh` in root)
2. **Creates** all required project-specific config files (if they don't already exist):
   - `.github/copilot-instructions.md` — project policies, commands, testing config
   - `.github/instructions/terminal-discipline.instructions.md` — CLI discipline rules
   - `.specify/memory/constitution.md` — project governance principles
   - `.specify/memory/agents.md` — command registry (agents resolve all commands from here)
  - `.specify/memory/bubbles.config.json` — control-plane defaults registry
  - `.specify/memory/.gitignore` + `.specify/metrics/.gitignore` + `.specify/runtime/.gitignore` — keep runtime profile/proposal/metrics/lease artifacts untracked
3. **Creates** the `specs/` directory for feature/bug specs
4. **Never overwrites** existing files — safe to re-run

Runtime-generated control-plane artifacts are created on demand and should remain untracked:
- `.specify/memory/developer-profile.md`
- `.specify/memory/skill-proposals.md`
- `.specify/memory/skill-proposals-dismissed.md`
- `.specify/metrics/*.jsonl`
- `.specify/runtime/resource-leases.json`
- `.specify/runtime/workflow-runs.json`
- `.specify/runtime/framework-events.jsonl`

After bootstrap, update the `TODO` items in the generated files, then start using agents.

### What Gets Installed (default shared install)

```
.github/
├── agents/
<!-- GENERATED:FRAMEWORK_STATS_INSTALL_TREE_START -->
│   ├── bubbles.workflow.agent.md    # 34 agent definitions
│   ├── bubbles.implement.agent.md
│   ├── bubbles.super.agent.md       # NEW: first-touch assistant + framework operations
│   ├── ...
│   └── bubbles_shared/              # Shared governance docs
│       ├── agent-common.md
│       ├── scope-workflow.md
│       └── ...
├── prompts/
│   └── bubbles.*.prompt.md          # 34 prompt shims
├── bubbles/
│   ├── workflows.yaml               # 29 workflow mode definitions
│   ├── scripts/                     # Governance scripts
│   │   ├── cli.sh                   # Main CLI
│   │   ├── artifact-lint.sh
│   │   ├── state-transition-guard.sh
│   │   └── ...
│   └── docs/                        # Generated docs
└── scripts/
    └── bubbles.sh                   # CLI shim (dispatches to bubbles/scripts/cli.sh)
<!-- GENERATED:FRAMEWORK_STATS_INSTALL_TREE_END -->
```

Use `--agents-only` if you want to skip the portable shared instructions and governance skills.

### What `--bootstrap` Adds (project-specific)

```
.github/
├── copilot-instructions.md              # Project policies & commands
├── instructions/
│   └── terminal-discipline.instructions.md  # CLI discipline
.specify/memory/
├── constitution.md                      # Project governance
└── agents.md                            # Command registry
specs/                                   # Feature/bug spec folders
```

---

## The Crew

<p align="center">
  <img src="pictures/bazaar_v5_agent_icons_presentation.svg" width="900" alt="Bubbles Agent Network Presentation Layout">
</p>

Every agent has a job. Start with `/bubbles.workflow` — it figures out which specialists to call. Use `/bubbles.super` for framework ops and advice.

### Artifact Ownership

Bubbles now enforces hard artifact ownership:

- `bubbles.analyst` owns business requirements in `spec.md`
- `bubbles.ux` owns UX sections inside `spec.md`
- `bubbles.design` owns `design.md`
- `bubbles.plan` owns `scopes.md`, `report.md` structure, `uservalidation.md`, and `scenario-manifest.json`
- `bubbles.validate` owns certification state in `state.json`
- `bubbles.docs` owns the managed docs declared in the effective managed-doc registry (framework defaults plus any project-owned overrides)
- Diagnostic and certification agents like `bubbles.validate`, `bubbles.audit`, `bubbles.harden`, `bubbles.gaps`, `bubbles.stabilize`, `bubbles.security`, `bubbles.regression`, `bubbles.clarify`, `bubbles.code-review`, and `bubbles.system-review` must route foreign-artifact changes to the owning specialist instead of editing those artifacts directly

Control-plane law:
- Orchestrators dispatch work and keep it moving; they do not implement fixes directly.
- Only orchestrators may invoke child workflows.
- Owners and execution specialists produce concrete code, test, doc, or artifact deltas.
- Diagnostic and certification agents finish with concrete result envelopes and owner-targeted packets instead of inline remediation.

This is enforced by the artifact ownership contract in `.github/agents/bubbles_shared/artifact-ownership.md`, the shared governance index in `.github/agents/bubbles_shared/agent-common.md`, the ownership manifest in `.github/bubbles/agent-ownership.yaml`, and the blocking `artifact_ownership_enforcement_gate` in `.github/bubbles/workflows.yaml`.

### Managed Docs And Ops Packets

- Published docs owned by Bubbles are declared in the effective managed-doc registry. Framework defaults live in `bubbles/docs-registry.yaml`, and project-owned overrides may live in `.github/bubbles-project.yaml`.
- Feature and bug packets remain execution truth while work is active; managed docs are the published truth.
- Cross-cutting infrastructure and operational delivery work lives under `specs/_ops/OPS-*`.
- Ops packets use `objective.md`, `design.md`, `scopes.md`, `runbook.md`, `report.md`, and `state.json`.

### <img src="icons/bubbles-glasses.svg" width="24"> Start Here — Universal Entry Point

| Icon | Agent | Role | When to Use |
|:----:|-------|------|-------------|
| <img src="icons/bubbles-glasses.svg" width="20"> | `bubbles.workflow` | **Universal entry point.** Accepts plain English, structured commands, or "continue". Resolves intent via `super`, picks work via `iterate`, drives all phases to completion. | **Always. Just describe what you want.** |
| <img src="icons/lahey-badge.svg" width="20"> | `bubbles.super` | **Framework ops & advice.** NLP resolver, command generator, framework health, framework validation, release hygiene, hooks, gates, upgrades, and repo-readiness guidance. Workflow delegates to it automatically for vague input. | Framework operations, advice without execution |

### <img src="icons/jacob-hardhat.svg" width="24"> Orchestrators

| Icon | Agent | Role | When to Use |
|:----:|-------|------|-------------|
| <img src="icons/jacob-hardhat.svg" width="20"> | `bubbles.iterate` | **Work picker.** Selects the highest-priority next slice and runs one iteration. Also accepts plain English via `super` delegation. | Continuing existing spec work without choosing phases by hand |
| <img src="icons/cory-cap.svg" width="20"> | `bubbles.bug` | **Bug orchestrator.** Reproduces, packets, routes, and drives the fix workflow until the defect is actually closed. | Investigating and routing bug work end to end |

### <img src="icons/julian-glass.svg" width="24"> Owners And Executors

| Icon | Agent | Role | When to Use |
|:----:|-------|------|-------------|
| <img src="icons/ray-lawnchair.svg" width="20"> | `bubbles.analyst` | **Business analyst.** Figures out the *why* behind requirements. | Starting new features |
| <img src="icons/lucy-mirror.svg" width="20"> | `bubbles.ux` | **UX designer.** Cares about how things feel and look. | UI/UX design work |
| <img src="icons/sarah-clipboard.svg" width="20"> | `bubbles.design` | **Architect.** Turns loose ideas into a crisp technical shape. | System design |
| <img src="icons/barb-keys.svg" width="20"> | `bubbles.plan` | **Scope planner.** Defines the scopes, owns planning artifacts, and keeps the books. | Breaking work into scopes |
| <img src="icons/julian-glass.svg" width="20"> | `bubbles.implement` | **The implementer.** Delivers every time. | Implementing planned scopes |
| <img src="icons/trinity-notebook.svg" width="20"> | `bubbles.test` | **Test verification.** Trusts nothing. Verifies everything. | Running/fixing test suites |
| <img src="icons/jroc-mic.svg" width="20"> | `bubbles.docs` | **Managed docs publisher.** Publishes the durable truth before closeout. | Updating published docs after changes |
| <img src="icons/ricky-dynamite.svg" width="20"> | `bubbles.chaos` | **Chaos tester.** Breaks things in ways nobody could predict. | Resilience testing |
| <img src="icons/donny-ducttape.svg" width="20"> | `bubbles.simplify` | **Simplifier.** Cuts through the noise without weakening behavior or ownership boundaries. | Reducing complexity after implementation |
| <img src="icons/tommy-rack.svg" width="20"> | `bubbles.devops` | **DevOps executor.** Owns CI/CD, build, deployment, monitoring, and observability execution once operational work is identified. | Shipping operational changes and delivery plumbing |
| <img src="icons/sebastian-guitar.svg" width="20"> | `bubbles.cinematic-designer` | **Premium UI implementer.** Over-the-top production value, real frontend output. | Cinematic or flagship UI implementation |

### <img src="icons/ted-badge.svg" width="24"> Diagnostic And Certification Routing

| Icon | Agent | Role | When to Use |
|:----:|-------|------|-------------|
| <img src="icons/randy-cheeseburger.svg" width="20"> | `bubbles.validate` | **Certification owner.** Checks the gates, owns certification state, and can reopen work with concrete packets. | Pre-merge validation and promotion gating |
| <img src="icons/ted-badge.svg" width="20"> | `bubbles.audit` | **Policy enforcer.** Final compliance pass that certifies or routes rework, not implementation. | Final compliance audit |
| <img src="icons/private-dancer-lamp.svg" width="20"> | `bubbles.grill` | **Pressure tester.** Interrogates ideas, plans, and assumptions before time gets wasted. | Challenging an idea or workflow choice up front |
| <img src="icons/george-green-badge.svg" width="20"> | `bubbles.clarify` | **Ambiguity router.** Identifies what is unclear, what is contradictory, and which owning agent must update the artifacts. | Resolving planning ambiguity without crossing ownership boundaries |
| <img src="icons/conky-puppet.svg" width="20"> | `bubbles.harden` | **Hardener.** Says the uncomfortable truths. Confrontational. Necessary. | Hardening passes |
| <img src="icons/phil-collins-baam.svg" width="20"> | `bubbles.gaps` | **Gap finder.** Finds what nobody else sees. | Gap analysis |
| <img src="icons/bill-wrench.svg" width="20"> | `bubbles.stabilize` | **Stabilizer.** Quiet. Reliable. Surfaces reliability issues and routes the correct owner. | Stability issues |
| <img src="icons/steve-french-paw.svg" width="20"> | `bubbles.regression` | **Regression guardian.** Prowls the codebase catching cross-feature interference. | After implementation/bug fixes |
| <img src="icons/cyrus-sunglasses.svg" width="20"> | `bubbles.security` | **Security scanner.** Finds threats. Confrontational. | Security review |
| <img src="icons/green-bastard-outline.svg" width="20"> | `bubbles.code-review` | **Engineering-first code reviewer.** Reviews repositories, services, packages, modules, and paths strictly from a code perspective. | Reviewing code directly before deciding what to fix |
| <img src="icons/orangie-fishbowl.svg" width="20"> | `bubbles.system-review` | **Holistic system reviewer.** Orangie sees everything from the fishbowl. Reviews the whole system. | Reviewing what the system feels like, does, and implies as a whole |
| <img src="icons/gary-laser-eyes.svg" width="20"> | `bubbles.spec-review` | **Spec freshness auditor.** Checks whether artifacts still deserve trust before maintenance or execution. | Auditing stale or drifted specs |

### <img src="icons/camera-crew.svg" width="24"> Utilities

| Icon | Agent | Role | When to Use |
|:----:|-------|------|-------------|
| <img src="icons/camera-crew.svg" width="20"> | `bubbles.status` | **Observer.** Reports state. Never interferes. Read-only. | Checking progress |
| <img src="icons/camera-crew.svg" width="20"> | `bubbles.recap` | **Talking head.** Summarizes what happened in this session, what is in progress, and what comes next. | Quick conversation recap |
| <img src="icons/lahey-bottle.svg" width="20"> | `bubbles.retro` | **Retrospective analyst.** Velocity metrics, gate health trends, hotspot analysis, and shipping patterns across sessions and specs. | Post-session or post-sprint retrospectives |
| <img src="icons/trevor-handoff.svg" width="20"> | `bubbles.handoff` | **Session handoff.** Packages context for the next session. | End of session |
| <img src="icons/cory-trevor-smokes.svg" width="20"> | `bubbles.setup` | **Framework setup.** Sets up or refreshes Bubbles project configuration and `.github` assets. | First-time project setup and framework refresh |
| <img src="icons/t-cap.svg" width="20"> | `bubbles.commands` | **Command registry.** Manages the project command reference. | Updating command docs |
| <img src="icons/sam-binoculars.svg" width="20"> | `bubbles.create-skill` | **Skill creator.** Packages know-how into reusable tools and playbooks. | Adding new skills |

---

## Quick Start

### 0. Setup (after install)
```
/bubbles.super  doctor                — Check framework health
/bubbles.commands                     — Auto-detect project, generate command registry
/bubbles.setup mode: refresh          — Verify framework setup completeness
```

### 1. Just Tell Workflow What You Want

**You don't need to know which agent, mode, or parameters to use. Workflow figures it out.**

```
# Describe your goal in plain English — workflow resolves the right mode and drives it:
/bubbles.workflow  Build a user authentication system with JWT tokens
/bubbles.workflow  improve the booking feature to be competitive
/bubbles.workflow  fix the calendar bug in page builder
/bubbles.workflow  spend 2 hours on whatever needs attention

# Continue from where you left off:
/bubbles.workflow  continue

# Or use structured mode when you know exactly what you want:
/bubbles.workflow  specs/042 mode: full-delivery tdd: true
```

### 1.5. The Most Useful Real-World Patterns

These are the most direct ways users interact with the newer planning and completion improvements.

```
# Explore the idea before any code is written
/bubbles.workflow  mode: brainstorm for multi-tenant booking search with competitive differentiation

# Improve a brownfield feature — objective research runs automatically
/bubbles.workflow  improve the booking feature to be competitive

# Fix a bug in existing code — reproduce/fix/verify loop with the quality chain
/bubbles.workflow  fix the calendar bug in page builder

# Keep shipping the next important slice without choosing phases by hand
/bubbles.workflow  continue

# Release-candidate / no-loose-ends delivery
/bubbles.workflow  specs/042-catalog-assistant mode: delivery-lockdown

# Measure rework and hotspot churn after a run
/bubbles.retro  week

# Framework-maintainer check for prompt bloat
bash bubbles/scripts/cli.sh lint-budget
```

| Command Pattern | What Bubbles Does For You |
|----------------|---------------------------|
| `mode: brainstorm` | Explores the idea without code and produces reviewable planning artifacts |
| `improve ...` / `mode: improve-existing` | Runs objective brownfield research, then produces/refines design and scopes before coding |
| `fix ...` / `mode: bugfix-fastlane` | Runs the focused bug loop with reproduce-before and verify-after evidence |
| `continue` | Resumes the active workflow if possible; otherwise falls back to `iterate` to pick the next highest-value work |
| `mode: delivery-lockdown` | Keeps looping through implementation, tests, quality sweep, validation, and audit until the feature is truly green or concretely blocked |
| `/bubbles.retro ...` | Shows slop tax and hotspot data so you can see whether you are shipping progress or just cleaning up rework |

### 2. How It Works Under The Hood

Workflow's Phase -1 classifies your input and delegates:

| Your Input | What Happens |
|-----------|-------------|
| Plain English | Delegates to `super` for NLP resolution → gets mode + spec + tags → executes |
| "Continue" / "next" | Delegates to `iterate` for work-picking → gets next priority item → executes |
| Structured (`mode:` + spec) | Skips resolution, executes phases directly |
| Framework ops ("doctor", "hooks") | Delegates to `super` for framework operations |

### 2.5. How The Planning Improvements Show Up In Practice

You usually do not invoke these as separate commands. They show up as workflow behavior and short reviewable artifacts.

| Improvement | How Users Experience It |
|------------|--------------------------|
| **Objective Research Pass** | Brownfield modes run a two-pass research step before design so the workflow captures current truth instead of jumping straight to solution-shaped opinions |
| **Design Brief** | `design.md` starts with a short alignment checkpoint you can review in a few minutes instead of reading a giant design doc |
| **Execution Outline** | `scopes.md` starts with a short plan preamble so you can steer the order and checkpoints before implementation |
| **Horizontal Plan Detection** | If planning drifts into DB → service → API → UI sequencing, Bubbles restructures toward vertical slices |
| **Slop Tax** | `bubbles.retro` reports rework signals like retries, reversions, and fix-on-fix churn |
| **Instruction Budget Lint** | Framework maintainers can audit prompt size with `bubbles lint-budget` instead of guessing when prompts got too bloated |

### 2.6. Review The Short Artifacts, Then Read The Code

The intended loop is:

1. Run a workflow.
2. Review the **Design Brief** at the top of `design.md`.
3. Review the **Execution Outline** at the top of `scopes.md`.
4. Let Bubbles implement.
5. Read the actual code and test evidence.

The short artifacts are there to help you steer early. They are not a substitute for reading the implementation.

### 3. Direct Agents (When You Know The Target)

You can still call any specialist directly when you explicitly want surgical work. Recap, status, and handoff continuation should usually go back through `/bubbles.workflow ...` instead of jumping straight to implement/test/validate.

```
/bubbles.analyst   Build a user authentication system with JWT tokens
/bubbles.implement Execute scope 1 of auth
/bubbles.test      Run all tests for the auth feature
/bubbles.code-review  profile: engineering-sweep scope: path:services/gateway
/bubbles.system-review  mode: full scope: feature:auth output: summary-doc
/bubbles.super     help me choose the right workflow mode
```

---

## Workflow Modes

<!-- GENERATED:FRAMEWORK_STATS_WORKFLOW_INTRO_START -->
Bubbles supports 29 workflow modes plus optional execution tags. Here are the most common:
<!-- GENERATED:FRAMEWORK_STATS_WORKFLOW_INTRO_END -->

| Mode | What It Does | Use When |
|------|-------------|----------|
| `full-delivery` | All phases: analyze → design → plan → implement → test → regression → simplify → stabilize → devops → security → validate → audit → docs | New features |
| `delivery-lockdown` | Repeat the full improvement + certification chain until validate can certify done | Release-candidate or legacy hardening delivery |
| `bugfix-fastlane` | Reproduce → fix → test → regression → simplify → stabilize → devops → security → validate → audit | Bug fixes |
| `value-first-e2e-batch` | Prioritized delivery with the full quality chain per batch | Large features |
| `chaos-hardening` | Chaos → fix → regression → hardening → validate → audit | Resilience work |
| `harden-gaps-to-doc` | Harden → gaps → test → docs | Quality sweeps |
| `devops-to-doc` | DevOps → test → stabilize → validate → docs | Operational delivery work |
| `simplify-to-doc` | Simplify → test → validate → audit → docs | Safe cleanup of existing implementations |
| `retro-quality-sweep` | Retro-guided simplify/harden quality sweep | Hotspot-driven maintenance |
| `stochastic-quality-sweep` | Random quality checks across the codebase | Periodic maintenance |

<!-- GENERATED:FRAMEWORK_STATS_WORKFLOW_OUTRO_START -->
See [docs/guides/WORKFLOW_MODES.md](docs/guides/WORKFLOW_MODES.md) for all 29 modes.
<!-- GENERATED:FRAMEWORK_STATS_WORKFLOW_OUTRO_END -->

For engineering-only code review work that should not enter the spec workflow, use `bubbles.code-review` with a review profile from `bubbles/code-review.yaml`.

For holistic feature, component, journey, or system review, use `bubbles.system-review` with a mode from `bubbles/system-review.yaml`.

Optional execution tags:
- `grillMode: required-on-ambiguity` pressure-tests the direction before planning or implementation starts.
- `tdd: true` forces a red-green-first execution loop inside the implement/test path after planning readiness is already satisfied.
- `backlogExport: tasks|issues` makes planning emit copy-ready backlog outputs per scope.
- `improvementPrelude: analyze-design-plan|analyze-ux-design-plan` turns on delivery-lockdown pre-round planning passes.
- `improvementPreludeRounds: N` limits how many delivery-lockdown rounds may run that prelude.
- `specReview: once-before-implement` runs one freshness/redundancy audit before legacy improvement or implementation work starts so stale active specs are reconciled once, not rediscovered every retry round.

Baseline workflow law already requires coherent spec/design/plan artifacts, explicit Gherkin scenarios, scenario-specific test planning, and scenario-driven E2E/integration proof before implementation begins.
- `socratic: true` turns on a bounded clarification loop before discovery/bootstrap work.
- `gitIsolation: true` opts into branch/worktree isolation when project policy allows it.
- `autoCommit: scope|dod` opts into atomic commits after validated milestones.
- `maxScopeMinutes` and `maxDodMinutes` tighten planning so scopes stay small and isolated.
- `microFixes: true` keeps failure recovery in narrow error-scoped loops.

Control-plane rules:
- Every specialist invocation ends with a concrete result envelope: `completed_owned`, `completed_diagnostic`, `route_required`, or `blocked`.
- Route-required outcomes carry owner-targeted packets with scope, scenario, or DoD references.
- Diagnostic and certification phases route foreign-owned follow-up; they do not perform inline remediation.
- Child workflows are orchestrator-only and bounded in depth.
- `scenario-manifest.json` carries stable `SCN-*` contracts, and validate replays linked live-system scenario proof before certification.
- `uservalidation.md` remains human acceptance input; automation findings do not toggle it.

Use `/bubbles.super` for framework operations (doctor, hooks, upgrade, metrics) or when you want command recommendations without execution. Workflow delegates to super automatically for vague input.

---

## The Rules

Bubbles enforces a strict quality system. This isn't optional.

### Zero-Fabrication Policy
Every piece of evidence must come from **actual terminal execution**. Writing "tests pass" without running tests is fabrication. Fabrication is detected and rejected.

<!-- GENERATED:FRAMEWORK_STATS_GATES_HEADING_START -->
### 60 Quality Gates
<!-- GENERATED:FRAMEWORK_STATS_GATES_HEADING_END -->
Every scope must pass all applicable gates before completion. Gates check everything from test coverage to evidence integrity to DoD completeness.

### Artifact Ownership Gate (G042)
Cross-authoring is blocked. If a diagnostic or downstream specialist finds that a foreign-owned artifact must change, it must route that work to the owning specialist instead of rewriting the artifact directly.

### Downstream Framework Ownership
Consumer repos may install and refresh Bubbles, but they must not author direct edits to framework-managed Bubbles files. Record requested framework changes in `.github/bubbles-project/proposals/` with `bubbles framework-proposal <slug>`, implement the real change in the Bubbles source repo, then refresh downstream installs.

### Self-Healing Loops (G039)
When agents hit failures, they attempt bounded self-repair: narrow context, retry up to 3 times, never stack. No infinite loops.

### Zero Deferral
Every issue found is fixed **now**. "We'll fix it later" is not a valid state. If a gate fails, work stops until it's resolved.

### Zero Warnings
Build, lint, and test output must produce zero warnings. Warnings are errors.

---

## Docs

| Document | What's Inside |
|----------|--------------|
| [It's Not Rocket Appliances](https://pkirsanov.github.io/bubbles/docs/its-not-rocket-appliances.html) | Visual agent reference card — rendered on GitHub Pages |
| [Cheatsheet](docs/CHEATSHEET.md) | Markdown quick-reference |
| [Agent Manual](docs/guides/AGENT_MANUAL.md) | Detailed guide for every agent |
<!-- GENERATED:CAPABILITY_LEDGER_DOCS_ROW_START -->
| [Competitive Capabilities](docs/generated/competitive-capabilities.md) | Ledger-backed competitive posture guide — 4 shipped, 1 partial, 2 proposed |
| [Issue Status](docs/generated/issue-status.md) | Ledger-backed status for 2 tracked framework gaps and proposals |
| [Interop Migration Matrix](docs/generated/interop-migration-matrix.md) | Ledger + registry-backed migration matrix for Claude Code, Roo Code, Cursor, and Cline |
<!-- GENERATED:CAPABILITY_LEDGER_DOCS_ROW_END -->
<!-- GENERATED:FRAMEWORK_STATS_DOCS_ROW_START -->
| [Workflow Modes](docs/guides/WORKFLOW_MODES.md) | All 29 workflow modes explained |
<!-- GENERATED:FRAMEWORK_STATS_DOCS_ROW_END -->
| [Interop Migration Guide](docs/guides/INTEROP_MIGRATION.md) | Supported apply, review-only intake, and proposal-only migration paths for external rule ecosystems |
| [Control Plane Design](docs/guides/CONTROL_PLANE_DESIGN.md) | Proposed architecture for registry-driven delegation, validate-owned certification, lockdown, and scenario contracts |
| [Control Plane Rollout](docs/guides/CONTROL_PLANE_ROLLOUT.md) | Phased implementation plan for the control-plane redesign across all requested changes |
| [Control Plane Schemas](docs/guides/CONTROL_PLANE_SCHEMAS.md) | Proposed schema set for capability registry, policy defaults, scenario manifests, certification state, and rework packets |
| [Recipes](docs/recipes/) | Common problems → solutions |
| [Installing in Your Repo](docs/guides/INSTALLATION.md) | Step-by-step setup guide |
| [Spec Examples](docs/examples/) | Annotated reference examples for common patterns |
| [Shared Skills](skills/) | Portable governance skills installed to every repo |

---

## Recipes (Quick Reference)

> "Boys, we need a plan." — Here's what to type.

**Start with `/bubbles.workflow` — it handles everything:**

| I Want To... | Run This |
|-------------|----------|
| **Just describe what I want** | **`/bubbles.workflow  <describe it in plain English>`** |
| **Continue where I left off** | **`/bubbles.workflow  continue`** |
| Explore an idea before writing code | `/bubbles.workflow  mode: brainstorm for <idea>` |
| Start a new feature from scratch | `/bubbles.workflow  <describe feature>` |
| Improve an existing feature | `/bubbles.workflow  improve <feature>` |
| Fix a bug | `/bubbles.workflow  fix the <describe bug>` |
| Run the full delivery pipeline | `/bubbles.workflow  full-delivery for <feature>` |
| Reconcile and redesign | `/bubbles.workflow  redesign-existing for <feature>` |
| Harden the code quality | `/bubbles.workflow  harden <feature>` |
| Break things on purpose | `/bubbles.workflow  chaos test <feature>` |
| Spend time on whatever | `/bubbles.workflow  spend 2 hours on whatever needs attention` |
| Maximum assurance delivery | `/bubbles.workflow  <feature> mode: delivery-lockdown` |
| Show rework and hotspot patterns | `/bubbles.retro  week` |

**Direct agents (when you know the target):**

| I Want To... | Run This |
|-------------|----------|
| Review code directly | `/bubbles.code-review  scope: full-repo output: summary-only` |
| Review a feature holistically | `/bubbles.system-review  mode: full scope: component:<name>` |
| Check what's going on | `/bubbles.status` |
| Something's not right, validate it | `/bubbles.workflow  <feature> mode: validate-to-doc` |
| Hand off to next session | `/bubbles.handoff` |

**Framework operations:**

| I Want To... | Run This |
|-------------|----------|
| Check project health | `/bubbles.super  doctor` |
| Install git hooks | `/bubbles.super  install hooks` |
| Upgrade bubbles | `/bubbles.super  upgrade` |
| Add a custom quality gate | `/bubbles.super  add a pre-push gate for license checking` |
| View scope dependencies | `/bubbles.super  show dag for 042` |
| Get help choosing a mode | `/bubbles.super  help me <describe goal>` |

See [docs/recipes/](docs/recipes/) for detailed step-by-step guides.

---

## Project Structure

```
bubbles/
<!-- GENERATED:FRAMEWORK_STATS_PROJECT_TREE_START -->
├── agents/                    # 34 agent definitions
│   ├── bubbles_shared/        # Shared governance docs
│   ├── bubbles.workflow.agent.md
│   ├── bubbles.implement.agent.md
│   ├── bubbles.super.agent.md # NEW: first-touch assistant + framework operations
│   └── ...
├── prompts/                   # 34 prompt shims
<!-- GENERATED:FRAMEWORK_STATS_PROJECT_TREE_END -->
├── bubbles/                   # Workflow config + scripts + generated docs
│   ├── workflows.yaml
│   ├── scripts/               # Governance scripts (artifact-lint, guard, etc.)
│   └── docs/                  # Generated docs (regenerated on upgrade)
├── templates/                 # Bootstrap templates for project setup
├── icons/                     # SVG icons for all agents
├── docs/
│   ├── its-not-rocket-appliances.html
│   ├── guides/                # Detailed documentation
│   ├── recipes/               # Problem → solution guides
│   └── examples/              # Annotated reference specs
├── install.sh                 # One-line installer (supports --bootstrap)
└── VERSION
```

---

## Contributing

1. Fork the repo
2. Make changes to agents/prompts/scripts
3. Test in at least one consumer repo
4. PR with description of what changed

Agent files are Markdown. The system is pure text. No build step. No compilation.

**Rule:** All agent files (`bubbles.*.agent.md`) must be project-agnostic. Zero repo-specific paths, commands, or tool references.

**Enforcement:** Run `bubbles agnosticity` for a full portable-surface drift check, `bubbles agnosticity --staged` for pre-commit scope, and `bubbles hooks install --all` to wire those checks into local git hooks.

---

## License

MIT — See [LICENSE](LICENSE).

---

<p align="center">
  <img src="icons/bubbles-glasses.svg" width="40">
  <br>
  <em>"Have a good one, boys."</em>
</p>
